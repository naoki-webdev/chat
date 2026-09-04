package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	postgresRealtimeChannel         = "orbit_chat_events"
	postgresNotifyPayloadLimitBytes = 7900
	postgresEventReplayBatchSize    = 100
	postgresListenerRetryDelay      = time.Second
)

// postgresRealtimeNotification keeps NOTIFY payloads small. Persisted events
// are identified by sequence and loaded from chat_events. Ephemeral events
// such as typing and presence are carried directly because they have no
// durable row to replay.
type postgresRealtimeNotification struct {
	Sequence  int64          `json:"sequence,omitempty"`
	Type      string         `json:"type,omitempty"`
	MessageID string         `json:"message_id,omitempty"`
	Event     *realtimeEvent `json:"event,omitempty"`
}

func (r *postgresRepository) startEventListener(handler func(realtimeEvent)) error {
	r.listenerMu.Lock()
	if r.listenerDone != nil {
		readyContext, cancel := context.WithTimeout(context.Background(), realtimeRepositoryTimeout)
		defer cancel()
		ready := r.listenerReady
		r.listenerMu.Unlock()
		return waitForEventListenerChannel(readyContext, ready)
	}
	listenerContext, cancel := context.WithCancel(context.Background())
	r.listenerCancel = cancel
	r.listenerDone = make(chan struct{})
	r.listenerReady = make(chan struct{})
	ready := r.listenerReady
	r.listenerMu.Unlock()
	go r.eventListenerLoop(listenerContext, handler)

	readyContext, readyCancel := context.WithTimeout(context.Background(), realtimeRepositoryTimeout)
	defer readyCancel()
	return waitForEventListenerChannel(readyContext, ready)
}

func waitForEventListenerChannel(ctx context.Context, ready <-chan struct{}) error {
	if ready == nil {
		return errors.New("postgres event listener is not started")
	}
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *postgresRepository) waitForEventListener(ctx context.Context) error {
	r.listenerMu.Lock()
	ready := r.listenerReady
	r.listenerMu.Unlock()
	return waitForEventListenerChannel(ctx, ready)
}

func (r *postgresRepository) eventListenerLoop(ctx context.Context, handler func(realtimeEvent)) {
	defer close(r.listenerDone)
	var lastSequence int64
	initialized := false

	for ctx.Err() == nil {
		connection, err := r.pool.Acquire(ctx)
		if err != nil {
			waitForPostgresListenerRetry(ctx)
			continue
		}
		firstConnection := !initialized

		if _, err := connection.Exec(ctx, "LISTEN "+postgresRealtimeChannel); err != nil {
			connection.Release()
			log.Printf("could not listen for realtime events: %v", err)
			waitForPostgresListenerRetry(ctx)
			continue
		}

		if !initialized {
			lastSequence, err = r.currentEventSequence(ctx)
			initialized = err == nil
		} else {
			err = r.broadcastPersistedEvents(ctx, lastSequence, handler, &lastSequence)
		}
		if err != nil {
			connection.Release()
			log.Printf("could not initialize realtime event listener: %v", err)
			waitForPostgresListenerRetry(ctx)
			continue
		}
		r.listenerReadyOnce.Do(func() {
			r.listenerMu.Lock()
			if r.listenerReady != nil {
				close(r.listenerReady)
			}
			r.listenerMu.Unlock()
		})

		for ctx.Err() == nil {
			notification, waitErr := connection.Conn().WaitForNotification(ctx)
			if waitErr != nil {
				if ctx.Err() == nil {
					log.Printf("realtime event listener disconnected: %v", waitErr)
				}
				break
			}
			notice, decodeErr := decodePostgresRealtimeNotification(notification.Payload)
			if decodeErr != nil {
				log.Printf("ignoring invalid realtime notification: %v", decodeErr)
				continue
			}
			if notice.Event != nil {
				handler(*notice.Event)
				continue
			}
			if notice.Sequence <= 0 {
				continue
			}

			// AI completion carries the temporary client-side message ID. It
			// shares the persisted sequence with message.created, so it must be
			// delivered even after the base event has advanced lastSequence.
			if notice.Type != "" {
				event, eventErr := r.eventBySequence(ctx, notice.Sequence)
				if eventErr != nil {
					log.Printf("could not load realtime event %d: %v", notice.Sequence, eventErr)
					continue
				}
				event.Type = notice.Type
				if notice.MessageID != "" {
					event.MessageID = notice.MessageID
				}
				handler(event)
				continue
			}
			if notice.Sequence <= lastSequence {
				if !firstConnection {
					continue
				}
				event, eventErr := r.eventBySequence(ctx, notice.Sequence)
				if eventErr != nil {
					log.Printf("could not load startup realtime event %d: %v", notice.Sequence, eventErr)
					continue
				}
				handler(event)
				continue
			}
			firstConnection = false
			if err := r.broadcastPersistedEvents(ctx, lastSequence, handler, &lastSequence); err != nil {
				log.Printf("could not replay realtime events: %v", err)
				break
			}
		}
		connection.Release()
		if ctx.Err() == nil {
			waitForPostgresListenerRetry(ctx)
		}
	}
}

func waitForPostgresListenerRetry(ctx context.Context) {
	timer := time.NewTimer(postgresListenerRetryDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (r *postgresRepository) currentEventSequence(ctx context.Context) (int64, error) {
	queryContext, cancel := context.WithTimeout(ctx, realtimeRepositoryTimeout)
	defer cancel()
	var sequence int64
	if err := r.pool.QueryRow(queryContext, `SELECT COALESCE(max(sequence),0) FROM chat_events`).Scan(&sequence); err != nil {
		return 0, err
	}
	return sequence, nil
}

func (r *postgresRepository) broadcastPersistedEvents(ctx context.Context, after int64, handler func(realtimeEvent), lastSequence *int64) error {
	for {
		queryContext, cancel := context.WithTimeout(ctx, realtimeRepositoryTimeout)
		events, err := r.listPersistedEvents(queryContext, after, postgresEventReplayBatchSize)
		cancel()
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		for _, event := range events {
			handler(event)
			after = event.Sequence
			*lastSequence = after
		}
		if len(events) < postgresEventReplayBatchSize {
			return nil
		}
	}
}

func (r *postgresRepository) listPersistedEvents(ctx context.Context, after int64, limit int) ([]realtimeEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT sequence,event_type,channel_id,COALESCE(message_id,''),COALESCE(parent_message_id,''),COALESCE(member_id,''),payload FROM chat_events WHERE sequence>$1 ORDER BY sequence LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]realtimeEvent, 0, limit)
	for rows.Next() {
		var sequence int64
		var eventType, channelID, messageID, parentMessageID, memberID string
		var payload []byte
		if err := rows.Scan(&sequence, &eventType, &channelID, &messageID, &parentMessageID, &memberID, &payload); err != nil {
			return nil, err
		}
		event, err := decodePersistedRealtimeEvent(sequence, eventType, channelID, messageID, parentMessageID, memberID, payload)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *postgresRepository) eventBySequence(ctx context.Context, sequence int64) (realtimeEvent, error) {
	queryContext, cancel := context.WithTimeout(ctx, realtimeRepositoryTimeout)
	defer cancel()
	var eventType, channelID, messageID, parentMessageID, memberID string
	var payload []byte
	if err := r.pool.QueryRow(queryContext, `SELECT event_type,channel_id,COALESCE(message_id,''),COALESCE(parent_message_id,''),COALESCE(member_id,''),payload FROM chat_events WHERE sequence=$1`, sequence).Scan(&eventType, &channelID, &messageID, &parentMessageID, &memberID, &payload); err != nil {
		return realtimeEvent{}, err
	}
	event, err := decodePersistedRealtimeEvent(sequence, eventType, channelID, messageID, parentMessageID, memberID, payload)
	if err != nil {
		return realtimeEvent{}, err
	}
	return event, nil
}

func decodePostgresRealtimeNotification(payload string) (postgresRealtimeNotification, error) {
	var notification postgresRealtimeNotification
	if err := json.Unmarshal([]byte(payload), &notification); err != nil {
		return postgresRealtimeNotification{}, err
	}
	if notification.Sequence <= 0 && notification.Event == nil {
		return postgresRealtimeNotification{}, errors.New("notification has no event or sequence")
	}
	return notification, nil
}

func (r *postgresRepository) publishEphemeral(event realtimeEvent) error {
	payload, err := json.Marshal(postgresRealtimeNotification{Event: &event})
	if err != nil {
		return err
	}
	return r.publishNotification(payload)
}

func (r *postgresRepository) publishAICompleted(event realtimeEvent) error {
	payload, err := json.Marshal(postgresRealtimeNotification{Sequence: event.Sequence, Type: event.Type, MessageID: event.MessageID})
	if err != nil {
		return err
	}
	return r.publishNotification(payload)
}

func (r *postgresRepository) publishNotification(payload []byte) error {
	if len(payload) > postgresNotifyPayloadLimitBytes {
		return errors.New("realtime notification payload exceeds PostgreSQL limit")
	}
	queryContext, cancel := context.WithTimeout(context.Background(), realtimeRepositoryTimeout)
	defer cancel()
	_, err := r.pool.Exec(queryContext, `SELECT pg_notify($1, $2)`, postgresRealtimeChannel, string(payload))
	return err
}

func notifyPersistedEvent(ctx context.Context, transaction pgx.Tx, sequence int64) error {
	payload, err := json.Marshal(postgresRealtimeNotification{Sequence: sequence})
	if err != nil {
		return err
	}
	_, err = transaction.Exec(ctx, `SELECT pg_notify($1, $2)`, postgresRealtimeChannel, string(payload))
	return err
}
