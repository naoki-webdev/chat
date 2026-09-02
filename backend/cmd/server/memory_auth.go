package main

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (r *memoryRepository) RegisterUser(_ context.Context, request registerRequest) (User, error) {
	if err := validateRegistration(request); err != nil {
		return User{}, err
	}
	email := normalizeEmail(request.Email)
	hash, err := hashPassword(request.Password)
	if err != nil {
		return User{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byEmail[email]; exists {
		return User{}, ErrConflict
	}
	name := strings.TrimSpace(request.Name)
	baseHandle := handleFromName(name)
	if baseHandle == "" {
		baseHandle = "user"
	}
	handle := baseHandle
	for attempt := 1; ; attempt++ {
		available := true
		for _, other := range r.users {
			if other.Handle == handle {
				available = false
				break
			}
		}
		if available {
			break
		}
		handle = baseHandle + "-" + randomID()[:6]
		if attempt >= 8 {
			return User{}, ErrConflict
		}
	}
	user := User{ID: "u-" + randomID(), Name: name, Email: email, Handle: handle, Initials: initialsFromName(name), Color: "linear-gradient(135deg, #f3a683, #c56cf0)"}
	r.users[user.ID] = userRecord{User: user, PasswordHash: hash}
	r.byEmail[email] = user.ID
	for _, channel := range r.channels {
		if channel.Kind == "channel" && isDefaultPublicChannel(channel.ID) {
			if r.memberships[channel.ID] == nil {
				r.memberships[channel.ID] = make(map[string]string)
			}
			r.memberships[channel.ID][user.ID] = "member"
		}
	}
	return user, nil
}

func (r *memoryRepository) AuthenticateUser(_ context.Context, email, password string) (User, error) {
	r.mu.RLock()
	userID, exists := r.byEmail[normalizeEmail(email)]
	record := r.users[userID]
	r.mu.RUnlock()
	if !exists || record.IsBot || bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(password)) != nil {
		return User{}, ErrUnauthorized
	}
	return record.User, nil
}

func (r *memoryRepository) UpdateUserProfile(_ context.Context, userID string, request updateProfileRequest) (User, error) {
	if err := validateUserName(request.Name); err != nil {
		return User{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.users[userID]
	if !exists || record.IsBot {
		return User{}, ErrUnauthorized
	}

	name := strings.TrimSpace(request.Name)
	baseHandle := handleFromName(name)
	if baseHandle == "" {
		baseHandle = "user"
	}
	handle := baseHandle
	const maxHandleAttempts = 8
	for attempt := 0; attempt < maxHandleAttempts; attempt++ {
		available := true
		for otherID, other := range r.users {
			if otherID != userID && other.Handle == handle {
				available = false
				break
			}
		}
		if available {
			break
		}
		handle = baseHandle + "-" + randomID()[:6]
		if attempt == maxHandleAttempts-1 {
			return User{}, ErrConflict
		}
	}

	record.Name = name
	record.Handle = handle
	record.Initials = initialsFromName(name)
	r.users[userID] = record
	for index := range r.channels {
		if r.channels[index].Kind == "dm" && r.channels[index].PeerUserID == userID {
			r.channels[index].Name = record.Name
			r.channels[index].Initials = record.Initials
			r.channels[index].Color = record.Color
		}
	}
	for channelID, messages := range r.messages {
		for index := range messages {
			if r.owners[messages[index].ID] != userID {
				continue
			}
			messages[index].Author = name
			messages[index].Initials = record.Initials
			messages[index].Color = record.Color
		}
		r.messages[channelID] = messages
	}
	return record.User, nil
}

func (r *memoryRepository) FindUserBySession(_ context.Context, token string) (User, error) {
	r.mu.RLock()
	session, exists := r.sessions[tokenHash(token)]
	record := r.users[session.UserID]
	r.mu.RUnlock()
	if !exists || record.IsBot || time.Now().After(session.ExpiresAt) {
		return User{}, ErrUnauthorized
	}
	return record.User, nil
}

func (r *memoryRepository) CreateSession(_ context.Context, userID string) (string, error) {
	r.mu.RLock()
	_, exists := r.users[userID]
	r.mu.RUnlock()
	if !exists {
		return "", ErrNotFound
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.sessions[tokenHash(token)] = memorySession{UserID: userID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}
	r.mu.Unlock()
	return token, nil
}

func (r *memoryRepository) DeleteSession(_ context.Context, token string) error {
	r.mu.Lock()
	delete(r.sessions, tokenHash(token))
	r.mu.Unlock()
	return nil
}
