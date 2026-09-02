package main

import (
	"context"
	"sort"
)

func (r *memoryRepository) ListUsers(_ context.Context) ([]PublicUser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	users := make([]PublicUser, 0, len(r.users))
	for _, record := range r.users {
		if record.IsBot {
			continue
		}
		users = append(users, PublicUser{ID: record.ID, Name: record.Name, Handle: record.Handle, Initials: record.Initials, Color: record.Color})
	}
	sort.Slice(users, func(left, right int) bool {
		if users[left].Name == users[right].Name {
			return users[left].ID < users[right].ID
		}
		return users[left].Name < users[right].Name
	})
	return users, nil
}

func (r *memoryRepository) ListChannelMembers(_ context.Context, channelID string) ([]ChannelMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.hasChannelLocked(channelID) {
		return nil, ErrNotFound
	}
	members := make([]ChannelMember, 0, len(r.memberships[channelID]))
	for userID, role := range r.memberships[channelID] {
		record, exists := r.users[userID]
		if !exists {
			continue
		}
		members = append(members, ChannelMember{
			PublicUser: PublicUser{ID: record.ID, Name: record.Name, Handle: record.Handle, Initials: record.Initials, Color: record.Color},
			Role:       role,
			IsBot:      record.IsBot,
		})
	}
	sort.Slice(members, func(left, right int) bool {
		if members[left].Name == members[right].Name {
			return members[left].ID < members[right].ID
		}
		return members[left].Name < members[right].Name
	})
	return members, nil
}
