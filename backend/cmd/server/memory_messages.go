package main

import (
	"context"
	"strings"
	"time"
)

func (r *memoryRepository) ListMessagePage(_ context.Context, channelID, before string, limit int) (MessagePage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.hasChannelLocked(channelID) {
		return MessagePage{}, ErrNotFound
	}
	limit = normalizeLimit(limit)
	beforeSequence, err := cursorValue(before)
	if err != nil {
		return MessagePage{}, invalidInput("invalid cursor")
	}
	items := r.messages[channelID]
	eligible := make([]Message, 0, len(items))
	for _, item := range items {
		if item.ParentMessageID != "" {
			continue
		}
		item.Sequence = r.messageSequences[item.ID]
		if beforeSequence == 0 || item.Sequence < beforeSequence {
			eligible = append(eligible, item)
		}
	}
	start := len(eligible) - limit
	if start < 0 {
		start = 0
	}
	page := eligible[start:]
	result := make([]Message, len(page))
	for index, item := range page {
		result[index] = cloneMessage(item)
	}
	hasMore := start > 0
	nextCursor := ""
	if hasMore && len(result) > 0 {
		nextCursor = cursorString(result[0].Sequence)
	}
	return MessagePage{Messages: result, NextCursor: nextCursor, HasMore: hasMore, Cursor: r.sequence}, nil
}

func (r *memoryRepository) ListThreadPage(_ context.Context, parentMessageID, before string, limit int) (MessagePage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit = normalizeLimit(limit)
	beforeSequence, err := cursorValue(before)
	if err != nil {
		return MessagePage{}, invalidInput("invalid cursor")
	}
	parentChannelID := ""
	for channelID, messages := range r.messages {
		for _, message := range messages {
			if message.ID == parentMessageID {
				parentChannelID = channelID
				break
			}
		}
	}
	if parentChannelID == "" {
		return MessagePage{}, ErrNotFound
	}
	eligible := make([]Message, 0)
	for _, message := range r.messages[parentChannelID] {
		if message.ParentMessageID != parentMessageID {
			continue
		}
		message.Sequence = r.messageSequences[message.ID]
		if beforeSequence == 0 || message.Sequence < beforeSequence {
			eligible = append(eligible, message)
		}
	}
	start := len(eligible) - limit
	if start < 0 {
		start = 0
	}
	page := eligible[start:]
	result := make([]Message, len(page))
	for index, item := range page {
		result[index] = cloneMessage(item)
	}
	hasMore := start > 0
	nextCursor := ""
	if hasMore && len(result) > 0 {
		nextCursor = cursorString(result[0].Sequence)
	}
	return MessagePage{Messages: result, NextCursor: nextCursor, HasMore: hasMore, Cursor: r.sequence}, nil
}

func (r *memoryRepository) CreateMessage(_ context.Context, channelID, userID string, request messageRequest) (Message, EventRecord, error) {
	body := strings.TrimSpace(request.Body)
	if err := validateMessageBody(body); err != nil {
		return Message{}, EventRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasChannelLocked(channelID) {
		return Message{}, EventRecord{}, ErrNotFound
	}
	user, ok := r.users[userID]
	if !ok {
		return Message{}, EventRecord{}, ErrUnauthorized
	}
	parentMessageID := strings.TrimSpace(request.ParentMessageID)
	if parentMessageID != "" {
		parentFound := false
		for _, parent := range r.messages[channelID] {
			if parent.ID == parentMessageID {
				if parent.ParentMessageID != "" {
					return Message{}, EventRecord{}, ErrConflict
				}
				parentFound = true
				break
			}
		}
		if !parentFound {
			return Message{}, EventRecord{}, ErrNotFound
		}
	}
	r.sequence++
	message := Message{ID: "m-" + randomID(), ChannelID: channelID, AuthorID: userID, Author: user.Name, Initials: user.Initials, Color: user.Color, Body: body, Time: time.Now().Format("15:04"), Reactions: []Reaction{}, ParentMessageID: parentMessageID, Sequence: r.sequence}
	r.messages[channelID] = append(r.messages[channelID], message)
	if parentMessageID != "" {
		for index := range r.messages[channelID] {
			if r.messages[channelID][index].ID == parentMessageID {
				r.messages[channelID][index].ThreadCount++
				break
			}
		}
	}
	r.messageSequences[message.ID] = message.Sequence
	r.owners[message.ID] = userID
	event := EventRecord{Sequence: r.sequence, Event: realtimeEvent{Type: "message.created", ChannelID: channelID, EventID: r.sequence, Sequence: r.sequence, Message: pointerToMessage(message)}}
	r.events = append(r.events, event)
	return cloneMessage(message), cloneEventRecord(event), nil
}

func (r *memoryRepository) UpdateMessage(_ context.Context, messageID, userID, body string) (Message, EventRecord, error) {
	body = strings.TrimSpace(body)
	if err := validateMessageBody(body); err != nil {
		return Message{}, EventRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for channelID, messages := range r.messages {
		for index := range messages {
			if messages[index].ID != messageID {
				continue
			}
			if messages[index].Deleted {
				return Message{}, EventRecord{}, ErrConflict
			}
			if r.owners[messages[index].ID] != userID {
				return Message{}, EventRecord{}, ErrForbidden
			}
			messages[index].Body, messages[index].Edited = body, true
			r.messages[channelID] = messages
			r.sequence++
			event := EventRecord{Sequence: r.sequence, Event: realtimeEvent{Type: "message.updated", ChannelID: channelID, EventID: r.sequence, Sequence: r.sequence, Message: pointerToMessage(messages[index])}}
			r.events = append(r.events, event)
			return cloneMessage(messages[index]), cloneEventRecord(event), nil
		}
	}
	return Message{}, EventRecord{}, ErrNotFound
}

func (r *memoryRepository) DeleteMessage(_ context.Context, messageID, userID string) (string, EventRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for channelID, messages := range r.messages {
		for index, message := range messages {
			if message.ID != messageID {
				continue
			}
			if r.owners[message.ID] != userID {
				return "", EventRecord{}, ErrForbidden
			}
			if message.Deleted {
				return "", EventRecord{}, ErrConflict
			}
			if message.ParentMessageID == "" {
				messages[index].Deleted = true
				messages[index].Body = deletedMessageBody
				messages[index].Edited = false
				messages[index].Reactions = nil
				r.messages[channelID] = messages
				delete(r.reactionUsers, messageID)
				r.sequence++
				deletedMessage := cloneMessage(messages[index])
				event := EventRecord{Sequence: r.sequence, Event: realtimeEvent{Type: "message.deleted", ChannelID: channelID, EventID: r.sequence, Sequence: r.sequence, MessageID: messageID, Message: pointerToMessage(deletedMessage)}}
				r.events = append(r.events, event)
				return channelID, cloneEventRecord(event), nil
			}
			if message.ParentMessageID != "" {
				for parentIndex := range messages {
					if messages[parentIndex].ID == message.ParentMessageID && messages[parentIndex].ThreadCount > 0 {
						messages[parentIndex].ThreadCount--
						break
					}
				}
			}
			r.messages[channelID] = append(messages[:index], messages[index+1:]...)
			delete(r.owners, messageID)
			r.sequence++
			event := EventRecord{Sequence: r.sequence, Event: realtimeEvent{Type: "message.deleted", ChannelID: channelID, EventID: r.sequence, Sequence: r.sequence, MessageID: messageID, ParentMessageID: message.ParentMessageID}}
			r.events = append(r.events, event)
			return channelID, cloneEventRecord(event), nil
		}
	}
	return "", EventRecord{}, ErrNotFound
}
