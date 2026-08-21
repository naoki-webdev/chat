package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"realtime-chat/backend/internal/ai"
)

func shouldInvokeAI(channelID, body string) bool {
	if channelID == "orbit-ai" {
		return true
	}
	return strings.Contains(strings.ToLower(body), "@orbit")
}

const aiMinInterval = time.Second
const defaultAIDailyRequestLimit = 100

type aiDailyEntry struct {
	Day      string
	Requests int
}

func configuredAIDailyRequestLimit() int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("AI_DAILY_REQUEST_LIMIT")))
	if err != nil || value < 1 {
		return defaultAIDailyRequestLimit
	}
	return value
}

const (
	maxAIContextCharacters  = 32_000
	maxAIResponseCharacters = 20_000
)

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func aiContextBody(message Message) string {
	threadLabel := ""
	if message.ParentMessageID != "" {
		threadLabel = fmt.Sprintf(" thread_parent=%s", message.ParentMessageID)
	}
	return fmt.Sprintf("[message_id=%s%s] %s", message.ID, threadLabel, strings.TrimSpace(message.Body))
}

func (s *server) acquireAI(key string) bool {
	s.aiMu.Lock()
	defer s.aiMu.Unlock()
	now := time.Now()
	if s.aiInFlight[key] >= 1 || now.Sub(s.aiLastRun[key]) < aiMinInterval {
		return false
	}
	userID := strings.SplitN(key, ":", 2)[0]
	today := now.UTC().Format("2006-01-02")
	daily := s.aiDaily[userID]
	if daily.Day == today && daily.Requests >= s.aiDailyLimit {
		return false
	}
	s.aiInFlight[key]++
	s.aiLastRun[key] = now
	if daily.Day != today {
		daily = aiDailyEntry{Day: today}
	}
	daily.Requests++
	s.aiDaily[userID] = daily
	return true
}

func (s *server) releaseAI(key string) {
	s.aiMu.Lock()
	defer s.aiMu.Unlock()
	if s.aiInFlight[key] <= 1 {
		delete(s.aiInFlight, key)
		return
	}
	s.aiInFlight[key]--
}

func (s *server) startAIReply(channelID, userID string, userMessage Message) {
	if s.aiService == nil || userMessage.Author == "Orbit AI" {
		return
	}
	key := userID + ":" + channelID
	if !s.acquireAI(key) {
		log.Printf("Orbit AI request rate limited for user %s in channel %s", userID, channelID)
		return
	}
	defer s.releaseAI(key)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	history := make([]ai.Message, 0, 50)
	if messages, err := s.repository.ListAIContextMessages(ctx, channelID, 50); err == nil {
		contextCharacters := 0
		for _, message := range messages {
			if message.ID == userMessage.ID {
				continue
			}
			if contextCharacters >= maxAIContextCharacters {
				break
			}
			body := truncateRunes(aiContextBody(message), maxAIContextCharacters-contextCharacters)
			if body == "" {
				break
			}
			history = append(history, ai.Message{Author: message.Author, Body: body})
			contextCharacters += len([]rune(body))
			if contextCharacters >= maxAIContextCharacters {
				break
			}
		}
	} else {
		log.Printf("could not load AI context for channel %s: %v", channelID, err)
	}

	temporaryID := "ai-" + randomID()
	started := Message{
		ID:        temporaryID,
		ChannelID: channelID,
		Author:    "Orbit AI",
		Initials:  "✦",
		Color:     "linear-gradient(135deg, #8b5cf6, #22d3ee)",
		Time:      time.Now().Format("15:04"),
	}
	s.broadcast(realtimeEvent{Type: "message.ai_started", ChannelID: channelID, MessageID: temporaryID, Message: pointerToMessage(started)})

	responseCharacters := 0
	finalBody, err := s.aiService.Stream(ctx, history, userMessage.Body, func(delta string) error {
		if delta == "" {
			return nil
		}
		responseCharacters += len([]rune(delta))
		if responseCharacters > maxAIResponseCharacters {
			return errors.New("Orbit AI response exceeded the configured limit")
		}
		s.broadcast(realtimeEvent{Type: "message.ai_delta", ChannelID: channelID, MessageID: temporaryID, Delta: delta})
		return nil
	})
	if err != nil {
		log.Printf("Orbit AI stream failed: %v", err)
		s.broadcast(realtimeEvent{Type: "message.ai_failed", ChannelID: channelID, MessageID: temporaryID, Error: "Orbit AIの応答に失敗しました。しばらくしてから再試行してください。"})
		return
	}
	finalBody = strings.TrimSpace(finalBody)
	if finalBody == "" {
		s.broadcast(realtimeEvent{Type: "message.ai_failed", ChannelID: channelID, MessageID: temporaryID, Error: "Orbit AIから空の応答が返りました。"})
		return
	}

	finalMessage, record, err := s.repository.CreateMessage(ctx, channelID, orbitAIUserID, messageRequest{Body: finalBody})
	if err != nil {
		log.Printf("could not persist Orbit AI message: %v", err)
		s.broadcast(realtimeEvent{Type: "message.ai_failed", ChannelID: channelID, MessageID: temporaryID, Error: "Orbit AIの回答を保存できませんでした。"})
		return
	}
	// The final message is persisted as a normal message.created event, so a
	// reconnect can recover it. Live clients receive the richer completed event
	// and replace the temporary streaming message in place.
	s.broadcast(realtimeEvent{Type: "message.ai_completed", ChannelID: channelID, EventID: record.Sequence, Sequence: record.Sequence, MessageID: temporaryID, Message: pointerToMessage(finalMessage)})
}
