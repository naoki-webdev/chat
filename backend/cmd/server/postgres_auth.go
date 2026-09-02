package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

func (r *postgresRepository) RegisterUser(ctx context.Context, request registerRequest) (User, error) {
	if err := validateRegistration(request); err != nil {
		return User{}, err
	}
	email := normalizeEmail(request.Email)
	hash, err := hashPassword(request.Password)
	if err != nil {
		return User{}, err
	}
	name := strings.TrimSpace(request.Name)
	baseHandle := handleFromName(name)
	if baseHandle == "" {
		baseHandle = "user"
	}
	user := User{ID: "u-" + randomID(), Name: name, Email: email, Initials: initialsFromName(name), Color: "linear-gradient(135deg, #f3a683, #c56cf0)"}
	const maxHandleAttempts = 8
	for attempt := 0; attempt < maxHandleAttempts; attempt++ {
		user.Handle = baseHandle
		if attempt > 0 {
			user.Handle += "-" + randomID()[:6]
		}

		transaction, err := r.pool.Begin(ctx)
		if err != nil {
			return User{}, err
		}
		err = transaction.QueryRow(ctx, `INSERT INTO users (id,name,email,handle,initials,color,password_hash) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (handle) DO NOTHING RETURNING id`, user.ID, user.Name, user.Email, user.Handle, user.Initials, user.Color, hash).Scan(&user.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = transaction.Rollback(ctx)
			continue
		}
		if isUniqueViolation(err) {
			_ = transaction.Rollback(ctx)
			return User{}, ErrConflict
		}
		if err != nil {
			_ = transaction.Rollback(ctx)
			return User{}, err
		}
		for _, channelID := range defaultPublicChannelIDs {
			if _, err := transaction.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) SELECT id,$1,'member' FROM channels WHERE id=$2 AND kind='channel' ON CONFLICT (channel_id,user_id) DO NOTHING`, user.ID, channelID); err != nil {
				_ = transaction.Rollback(ctx)
				return User{}, err
			}
		}
		if err := transaction.Commit(ctx); err != nil {
			return User{}, err
		}
		return user, nil
	}
	return User{}, ErrConflict
}

func (r *postgresRepository) AuthenticateUser(ctx context.Context, email, password string) (User, error) {
	var user User
	var passwordHash string
	err := r.pool.QueryRow(ctx, `SELECT id,name,email,handle,initials,color,password_hash FROM users WHERE email=$1 AND is_bot=false`, normalizeEmail(email)).Scan(&user.ID, &user.Name, &user.Email, &user.Handle, &user.Initials, &user.Color, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	if bcryptCompare(passwordHash, password) != nil {
		return User{}, ErrUnauthorized
	}
	return user, nil
}

func (r *postgresRepository) UpdateUserProfile(ctx context.Context, userID string, request updateProfileRequest) (User, error) {
	if err := validateUserName(request.Name); err != nil {
		return User{}, err
	}

	name := strings.TrimSpace(request.Name)
	const maxHandleAttempts = 8
	baseHandle := handleFromName(name)
	if baseHandle == "" {
		baseHandle = "user"
	}
	for attempt := 0; attempt < maxHandleAttempts; attempt++ {
		candidate := baseHandle
		if attempt > 0 {
			candidate += "-" + randomID()[:6]
		}
		transaction, err := r.pool.Begin(ctx)
		if err != nil {
			return User{}, err
		}
		var existingUserID string
		if err := transaction.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 AND is_bot=false FOR UPDATE`, userID).Scan(&existingUserID); err != nil {
			_ = transaction.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				return User{}, ErrUnauthorized
			}
			return User{}, err
		}
		var user User
		err = transaction.QueryRow(ctx, `UPDATE users SET name=$1, handle=$2, initials=$3 WHERE id=$4 AND is_bot=false RETURNING id,name,email,handle,initials,color`, name, candidate, initialsFromName(name), userID).Scan(&user.ID, &user.Name, &user.Email, &user.Handle, &user.Initials, &user.Color)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = transaction.Rollback(ctx)
			return User{}, ErrUnauthorized
		}
		if isUniqueViolation(err) {
			_ = transaction.Rollback(ctx)
			continue
		}
		if err != nil {
			_ = transaction.Rollback(ctx)
			return User{}, err
		}
		if _, err := transaction.Exec(ctx, `UPDATE channels SET name=$1, initials=$2, color=$3 WHERE dm_peer_user_id=$4 AND kind='dm'`, user.Name, user.Initials, user.Color, user.ID); err != nil {
			_ = transaction.Rollback(ctx)
			return User{}, err
		}
		if err := transaction.Commit(ctx); err != nil {
			return User{}, err
		}
		return user, nil
	}
	return User{}, ErrConflict
}

func bcryptCompare(hash, password string) error {
	if hash == "" {
		return ErrUnauthorized
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func (r *postgresRepository) FindUserBySession(ctx context.Context, token string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `SELECT u.id,u.name,u.email,u.handle,u.initials,u.color FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at > now() AND u.is_bot=false`, tokenHash(token)).Scan(&user.ID, &user.Name, &user.Email, &user.Handle, &user.Initials, &user.Color)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	return user, err
}

func (r *postgresRepository) CreateSession(ctx context.Context, userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO sessions (token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, tokenHash(token), userID, time.Now().Add(7*24*time.Hour))
	return token, err
}

func (r *postgresRepository) DeleteSession(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(token))
	return err
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23503"
}
