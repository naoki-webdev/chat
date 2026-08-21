package main

import (
	"context"
	"strings"
)

func (r *memoryRepository) ListChannels(_ context.Context, userID string) ([]Channel, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Channel, 0, len(r.channels))
	for _, channel := range r.channels {
		if !r.isChannelMemberLocked(userID, channel.ID) {
			continue
		}
		channel.Unread = r.unreadLocked(userID, channel.ID)
		result = append(result, channel)
	}
	return result, r.sequence, nil
}

func (r *memoryRepository) HasChannel(_ context.Context, id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hasChannelLocked(id), nil
}

func (r *memoryRepository) IsChannelMember(_ context.Context, userID, channelID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.hasChannelLocked(channelID) {
		return false, ErrNotFound
	}
	return r.isChannelMemberLocked(userID, channelID), nil
}

func (r *memoryRepository) ListChannelMemberIDs(_ context.Context, channelID string) (map[string]struct{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.hasChannelLocked(channelID) {
		return nil, ErrNotFound
	}
	result := make(map[string]struct{}, len(r.memberships[channelID]))
	for userID := range r.memberships[channelID] {
		result[userID] = struct{}{}
	}
	return result, nil
}

func (r *memoryRepository) ChannelIDForMessage(_ context.Context, messageID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for channelID, messages := range r.messages {
		for _, message := range messages {
			if message.ID == messageID {
				return channelID, nil
			}
		}
	}
	return "", ErrNotFound
}

func (r *memoryRepository) CreateChannel(_ context.Context, userID string, request channelRequest) (Channel, error) {
	if err := validateChannelRequest(request); err != nil {
		return Channel{}, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return Channel{}, invalidInput("name is required")
	}
	id := channelIDFromName(name)
	if id == "" {
		return Channel{}, invalidInput("name must include a valid character")
	}
	group := strings.TrimSpace(request.Group)
	if group == "" {
		group = "Product"
	}
	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		kind = "channel"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hasChannelLocked(id) {
		return Channel{}, ErrConflict
	}
	for _, memberID := range request.MemberIDs {
		record, exists := r.users[memberID]
		if !exists || record.IsBot {
			return Channel{}, invalidInput("member_ids contains an unavailable user")
		}
	}
	channel := Channel{ID: id, Name: name, Group: group, Kind: kind, Description: strings.TrimSpace(request.Description)}
	r.channels = append(r.channels, channel)
	r.messages[id] = []Message{}
	r.memberships[id] = map[string]string{userID: "owner"}
	for _, memberID := range request.MemberIDs {
		if memberID != userID {
			r.memberships[id][memberID] = "member"
		}
	}
	if kind == "channel" {
		r.memberships[id][orbitAIUserID] = "member"
	}
	return channel, nil
}

func (r *memoryRepository) UpdateChannel(_ context.Context, channelID, userID string, request channelUpdateRequest) (Channel, error) {
	if err := validateChannelUpdateRequest(request); err != nil {
		return Channel{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasChannelLocked(channelID) {
		return Channel{}, ErrNotFound
	}
	role := r.memberships[channelID][userID]
	if role != "owner" && role != "admin" {
		return Channel{}, ErrForbidden
	}
	if request.MemberIDs != nil {
		for _, memberID := range request.MemberIDs {
			record, exists := r.users[memberID]
			if !exists || record.IsBot {
				return Channel{}, invalidInput("member_ids contains an unavailable user")
			}
		}
		desired := make(map[string]struct{}, len(request.MemberIDs))
		for _, memberID := range request.MemberIDs {
			desired[memberID] = struct{}{}
		}
		for memberID, memberRole := range r.memberships[channelID] {
			if memberRole == "owner" || memberRole == "admin" || r.users[memberID].IsBot {
				desired[memberID] = struct{}{}
			}
		}
		for memberID := range r.memberships[channelID] {
			if _, keep := desired[memberID]; !keep {
				delete(r.memberships[channelID], memberID)
			}
		}
		for memberID := range desired {
			if _, exists := r.memberships[channelID][memberID]; !exists {
				r.memberships[channelID][memberID] = "member"
			}
		}
	}
	for index := range r.channels {
		if r.channels[index].ID != channelID {
			continue
		}
		r.channels[index].Name = strings.TrimSpace(request.Name)
		r.channels[index].Description = strings.TrimSpace(request.Description)
		return r.channels[index], nil
	}
	return Channel{}, ErrNotFound
}

func (r *memoryRepository) MarkChannelRead(_ context.Context, userID, channelID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasChannelLocked(channelID) {
		return 0, ErrNotFound
	}
	if r.readStates[userID] == nil {
		r.readStates[userID] = make(map[string]int64)
	}
	latest := r.latestChannelSequenceLocked(channelID)
	r.readStates[userID][channelID] = latest
	if r.readMessageIDs[userID] == nil {
		r.readMessageIDs[userID] = make(map[string]string)
	}
	latestMessageID := ""
	for _, message := range r.messages[channelID] {
		if r.messageSequences[message.ID] == latest {
			latestMessageID = message.ID
			break
		}
	}
	r.readMessageIDs[userID][channelID] = latestMessageID
	return latest, nil
}
