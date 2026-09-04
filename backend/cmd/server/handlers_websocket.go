package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

func (s *server) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	user, err := s.currentUser(request)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	channelID := request.URL.Query().Get("channel_id")
	if channelID == "" {
		channelID = defaultChannelID
	}
	globalSubscription := channelID == "all" || channelID == "*"
	if !globalSubscription {
		exists, err := s.repository.HasChannel(request.Context(), channelID)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		if !exists {
			http.NotFound(writer, request)
			return
		}
		if !s.requireChannelMember(writer, request, user, channelID) {
			return
		}
	}
	connection, err := s.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	if globalSubscription {
		channelID = "*"
	}
	client := &client{connection: connection, channelID: channelID, user: user, server: s, hub: s.hub, send: make(chan []byte, 32), done: make(chan struct{})}
	s.hub.add(client)
	go client.writePump()
	client.readPump()
}

func (s *server) broadcast(event realtimeEvent) {
	if postgres, ok := s.repository.(*postgresRepository); ok {
		if event.Sequence > 0 {
			if event.Type == "message.ai_completed" {
				if err := postgres.publishAICompleted(event); err != nil {
					log.Printf("could not publish AI completion notification: %v", err)
					s.broadcastLocal(event)
				}
			}
			return
		}
		if err := postgres.publishEphemeral(event); err != nil {
			log.Printf("could not publish realtime notification: %v", err)
			s.broadcastLocal(event)
		}
		return
	}
	s.broadcastLocal(event)
}

func (s *server) broadcastLocal(event realtimeEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("could not encode realtime event: %v", err)
		return
	}
	var memberIDs map[string]struct{}
	if event.ChannelID != "*" {
		ctx, cancel := context.WithTimeout(context.Background(), realtimeRepositoryTimeout)
		memberIDs, err = s.repository.ListChannelMemberIDs(ctx, event.ChannelID)
		cancel()
		if err != nil {
			log.Printf("could not snapshot channel membership for %s: %v", event.ChannelID, err)
			return
		}
		if event.Type == "channel.member_removed" && event.MemberID != "" {
			memberIDs[event.MemberID] = struct{}{}
		}
	}
	s.hub.broadcast(event.ChannelID, payload, memberIDs)
}

func (s *server) requireChannelMember(writer http.ResponseWriter, request *http.Request, user User, channelID string) bool {
	member, err := s.repository.IsChannelMember(request.Context(), user.ID, channelID)
	if err != nil {
		writeRepositoryError(writer, err)
		return false
	}
	if !member {
		writeRepositoryError(writer, ErrNotMember)
		return false
	}
	return true
}

func (s *server) requireMessageMember(writer http.ResponseWriter, request *http.Request, user User, messageID string) (string, bool) {
	channelID, err := s.repository.ChannelIDForMessage(request.Context(), messageID)
	if err != nil {
		writeRepositoryError(writer, err)
		return "", false
	}
	if !s.requireChannelMember(writer, request, user, channelID) {
		return "", false
	}
	return channelID, true
}
