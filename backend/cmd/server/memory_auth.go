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
	user := User{ID: "u-" + randomID(), Name: name, Email: email, Handle: handleFromName(name), Initials: initialsFromName(name), Color: "linear-gradient(135deg, #f3a683, #c56cf0)"}
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
