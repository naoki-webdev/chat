package main

import (
	"context"
	"strings"
)

func (r *memoryRepository) AddReaction(_ context.Context, messageID, userID, emoji string) (Message, EventRecord, error) {
	emoji = strings.TrimSpace(emoji)
	if err := validateReactionEmoji(emoji); err != nil {
		return Message{}, EventRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[userID]; !ok {
		return Message{}, EventRecord{}, ErrUnauthorized
	}
	for channelID, messages := range r.messages {
		for index := range messages {
			if messages[index].ID != messageID {
				continue
			}
			if messages[index].Deleted {
				return Message{}, EventRecord{}, ErrConflict
			}
			if r.reactionUsers[messageID] == nil {
				r.reactionUsers[messageID] = make(map[string]map[string]struct{})
			}
			if r.reactionUsers[messageID][userID] == nil {
				r.reactionUsers[messageID][userID] = make(map[string]struct{})
			}
			if _, exists := r.reactionUsers[messageID][userID][emoji]; exists {
				result := cloneMessage(messages[index])
				setReactionState(&result, emoji, true)
				return result, EventRecord{}, nil
			}
			r.reactionUsers[messageID][userID][emoji] = struct{}{}
			incrementReaction(&messages[index], emoji)
			r.messages[channelID] = messages
			r.sequence++
			result := cloneMessage(messages[index])
			setReactionState(&result, emoji, true)
			eventMessage := cloneMessage(result)
			clearReactionState(&eventMessage)
			event := EventRecord{Sequence: r.sequence, Event: realtimeEvent{Type: "reaction.added", ChannelID: channelID, EventID: r.sequence, Sequence: r.sequence, Message: pointerToMessage(eventMessage)}}
			r.events = append(r.events, event)
			return result, cloneEventRecord(event), nil
		}
	}
	return Message{}, EventRecord{}, ErrNotFound
}

func (r *memoryRepository) RemoveReaction(_ context.Context, messageID, userID, emoji string) (Message, EventRecord, error) {
	emoji = strings.TrimSpace(emoji)
	if err := validateReactionEmoji(emoji); err != nil {
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
			userReactions := r.reactionUsers[messageID][userID]
			if _, exists := userReactions[emoji]; !exists {
				result := cloneMessage(messages[index])
				setReactionState(&result, emoji, false)
				return result, EventRecord{}, nil
			}
			delete(userReactions, emoji)
			decrementReaction(&messages[index], emoji)
			r.messages[channelID] = messages
			r.sequence++
			result := cloneMessage(messages[index])
			eventMessage := cloneMessage(result)
			clearReactionState(&eventMessage)
			event := EventRecord{Sequence: r.sequence, Event: realtimeEvent{Type: "reaction.removed", ChannelID: channelID, EventID: r.sequence, Sequence: r.sequence, Message: pointerToMessage(eventMessage)}}
			r.events = append(r.events, event)
			return result, cloneEventRecord(event), nil
		}
	}
	return Message{}, EventRecord{}, ErrNotFound
}
