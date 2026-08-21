package main

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (r *postgresRepository) ListChannels(ctx context.Context, userID string) ([]Channel, int64, error) {
	var cursor int64
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM chat_events`).Scan(&cursor); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
SELECT c.id, c.name, c.channel_group, c.kind, c.description, COALESCE(c.presence,''), COALESCE(c.initials,''), COALESCE(c.color,''),
       COALESCE((SELECT count(*) FROM messages unread_message WHERE unread_message.channel_id=c.id AND unread_message.created_sequence > COALESCE((SELECT last_read_sequence FROM channel_read_states WHERE user_id=$1 AND channel_id=c.id),0)),0)
	FROM channels c JOIN channel_members cm ON cm.channel_id=c.id AND cm.user_id=$1 ORDER BY c.created_at, c.id`, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	channels := make([]Channel, 0)
	for rows.Next() {
		var channel Channel
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Group, &channel.Kind, &channel.Description, &channel.Presence, &channel.Initials, &channel.Color, &channel.Unread); err != nil {
			return nil, 0, err
		}
		channels = append(channels, channel)
	}
	return channels, cursor, rows.Err()
}

func (r *postgresRepository) HasChannel(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channels WHERE id=$1)`, id).Scan(&exists)
	return exists, err
}

func (r *postgresRepository) IsChannelMember(ctx context.Context, userID, channelID string) (bool, error) {
	var member bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channel_members WHERE channel_id=$1 AND user_id=$2)`, channelID, userID).Scan(&member)
	return member, err
}

func (r *postgresRepository) ListChannelMemberIDs(ctx context.Context, channelID string) (map[string]struct{}, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM channel_members WHERE channel_id=$1`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memberIDs := make(map[string]struct{})
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		memberIDs[userID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memberIDs, nil
}

func (r *postgresRepository) ChannelIDForMessage(ctx context.Context, messageID string) (string, error) {
	var channelID string
	err := r.pool.QueryRow(ctx, `SELECT channel_id FROM messages WHERE id=$1`, messageID).Scan(&channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return channelID, err
}

func (r *postgresRepository) CreateChannel(ctx context.Context, userID string, request channelRequest) (Channel, error) {
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
	description := strings.TrimSpace(request.Description)
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return Channel{}, err
	}
	defer transaction.Rollback(ctx)
	var channel Channel
	err = transaction.QueryRow(ctx, `INSERT INTO channels (id,name,channel_group,kind,description) VALUES ($1,$2,$3,$4,$5) RETURNING id,name,channel_group,kind,description`, id, name, group, kind, description).Scan(&channel.ID, &channel.Name, &channel.Group, &channel.Kind, &channel.Description)
	if isUniqueViolation(err) {
		return Channel{}, ErrConflict
	}
	if err != nil {
		return Channel{}, err
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) VALUES ($1,$2,'owner')`, channel.ID, userID); err != nil {
		if isForeignKeyViolation(err) {
			return Channel{}, ErrUnauthorized
		}
		return Channel{}, err
	}
	if kind == "channel" {
		if _, err := transaction.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) VALUES ($1,$2,'member') ON CONFLICT (channel_id,user_id) DO NOTHING`, channel.ID, orbitAIUserID); err != nil {
			return Channel{}, err
		}
	}
	for _, memberID := range request.MemberIDs {
		var available bool
		if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id=$1 AND is_bot=false)`, memberID).Scan(&available); err != nil {
			return Channel{}, err
		}
		if !available {
			return Channel{}, invalidInput("member_ids contains an unavailable user")
		}
		if _, err := transaction.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) VALUES ($1,$2,'member') ON CONFLICT (channel_id,user_id) DO NOTHING`, channel.ID, memberID); err != nil {
			return Channel{}, err
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return Channel{}, err
	}
	return channel, nil
}

func (r *postgresRepository) UpdateChannel(ctx context.Context, channelID, userID string, request channelUpdateRequest) (Channel, error) {
	if err := validateChannelUpdateRequest(request); err != nil {
		return Channel{}, err
	}
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return Channel{}, err
	}
	defer transaction.Rollback(ctx)
	var channel Channel
	err = transaction.QueryRow(ctx, `
UPDATE channels c
SET name=$1, description=$2
FROM channel_members cm
WHERE c.id=$3 AND cm.channel_id=c.id AND cm.user_id=$4 AND cm.role IN ('owner','admin')
RETURNING c.id,c.name,c.channel_group,c.kind,c.description`, strings.TrimSpace(request.Name), strings.TrimSpace(request.Description), channelID, userID).Scan(&channel.ID, &channel.Name, &channel.Group, &channel.Kind, &channel.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if existsErr := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channels WHERE id=$1)`, channelID).Scan(&exists); existsErr != nil {
			return Channel{}, existsErr
		}
		if !exists {
			return Channel{}, ErrNotFound
		}
		return Channel{}, ErrForbidden
	}
	if err != nil {
		return Channel{}, err
	}
	if request.MemberIDs != nil {
		for _, memberID := range request.MemberIDs {
			var available bool
			if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id=$1 AND is_bot=false)`, memberID).Scan(&available); err != nil {
				return Channel{}, err
			}
			if !available {
				return Channel{}, invalidInput("member_ids contains an unavailable user")
			}
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM channel_members WHERE channel_id=$1 AND role='member' AND user_id<>$3 AND NOT (user_id=ANY($2::text[]))`, channelID, request.MemberIDs, orbitAIUserID); err != nil {
			return Channel{}, err
		}
		for _, memberID := range request.MemberIDs {
			if _, err := transaction.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) VALUES ($1,$2,'member') ON CONFLICT (channel_id,user_id) DO NOTHING`, channelID, memberID); err != nil {
				return Channel{}, err
			}
		}
		if channel.Kind == "channel" {
			if _, err := transaction.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) VALUES ($1,$2,'member') ON CONFLICT (channel_id,user_id) DO NOTHING`, channelID, orbitAIUserID); err != nil {
				return Channel{}, err
			}
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return Channel{}, err
	}
	return channel, nil
}
