package main

import (
	"sync"
	"time"
)

type memorySession struct {
	UserID    string
	ExpiresAt time.Time
}

type memoryRepository struct {
	mu               sync.RWMutex
	sequence         int64
	channels         []Channel
	messages         map[string][]Message
	messageSequences map[string]int64
	owners           map[string]string
	events           []EventRecord
	readStates       map[string]map[string]int64
	readMessageIDs   map[string]map[string]string
	reactionUsers    map[string]map[string]map[string]struct{}
	memberships      map[string]map[string]string
	users            map[string]userRecord
	byEmail          map[string]string
	sessions         map[string]memorySession
	aiDailyUsage     map[string]int
}
