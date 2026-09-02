package main

import (
	"context"
	"time"
)

func (r *memoryRepository) ConsumeAIDailyQuota(_ context.Context, userID string, day time.Time, limit int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := userID + ":" + day.UTC().Format("2006-01-02")
	if r.aiDailyUsage[key] >= limit {
		return false, nil
	}
	r.aiDailyUsage[key]++
	return true, nil
}
