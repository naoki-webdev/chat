package main

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

func (r *memoryRepository) hasChannelLocked(id string) bool {
	for _, channel := range r.channels {
		if channel.ID == id {
			return true
		}
	}
	return false
}

func (r *memoryRepository) isChannelMemberLocked(userID, channelID string) bool {
	_, ok := r.memberships[channelID][userID]
	return ok
}

func (r *memoryRepository) unreadLocked(userID, channelID string) int {
	readAt := int64(0)
	if states := r.readStates[userID]; states != nil {
		readAt = states[channelID]
	}
	if readAt == 0 {
		if states := r.readMessageIDs[userID]; states != nil {
			readAt = r.messageSequences[states[channelID]]
		}
	}
	count := 0
	for _, record := range r.events {
		if record.Sequence > readAt && record.Event.ChannelID == channelID && record.Event.Type == "message.created" {
			count++
		}
	}
	return count
}

func (r *memoryRepository) latestChannelSequenceLocked(channelID string) int64 {
	latest := int64(0)
	for _, record := range r.events {
		if record.Event.ChannelID == channelID && record.Event.Type == "message.created" && record.Sequence > latest {
			latest = record.Sequence
		}
	}
	return latest
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func cloneMessage(message Message) Message {
	message.Reactions = append([]Reaction(nil), message.Reactions...)
	return message
}

func incrementReaction(message *Message, emoji string) {
	for index := range message.Reactions {
		if message.Reactions[index].Emoji == emoji {
			message.Reactions[index].Count++
			return
		}
	}
	message.Reactions = append(message.Reactions, Reaction{Emoji: emoji, Count: 1})
}

func decrementReaction(message *Message, emoji string) {
	for index := range message.Reactions {
		if message.Reactions[index].Emoji != emoji {
			continue
		}
		message.Reactions[index].Count--
		if message.Reactions[index].Count <= 0 {
			message.Reactions = append(message.Reactions[:index], message.Reactions[index+1:]...)
		}
		return
	}
}

func setReactionState(message *Message, emoji string, reacted bool) {
	for index := range message.Reactions {
		if message.Reactions[index].Emoji == emoji {
			message.Reactions[index].Reacted = reacted
		}
	}
}

func clearReactionState(message *Message) {
	for index := range message.Reactions {
		message.Reactions[index].Reacted = false
	}
}

func pointerToMessage(message Message) *Message {
	cloned := cloneMessage(message)
	return &cloned
}

func cloneEvent(event realtimeEvent) realtimeEvent {
	if event.Message != nil {
		event.Message = pointerToMessage(*event.Message)
	}
	return event
}

func cloneEventRecord(record EventRecord) EventRecord {
	record.Event = cloneEvent(record.Event)
	return record
}

func newChannelID() string {
	return "ch-" + randomID()
}

func handleFromName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}

func initialsFromName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "U"
	}
	initials := []rune{[]rune(parts[0])[0]}
	if len(parts) > 1 {
		initials = append(initials, []rune(parts[len(parts)-1])[0])
	}
	return strings.ToUpper(string(initials))
}

func randomID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(bytes)
}
