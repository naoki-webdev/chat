package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
)

func shouldSeedDemoData() bool {
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("SEED_DEMO_DATA"))); value == "true" || value == "1" || value == "yes" {
		return true
	}
	environment := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	return environment == "development" || environment == "dev" || environment == "test"
}

func (r *postgresRepository) migrate(ctx context.Context) error {
	connection, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	const lockID int64 = 816274
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return err
	}
	defer connection.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockID)

	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied bool
		if err := connection.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, entry.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		migration, err := fs.ReadFile(migrationFS, "migrations/"+entry.Name())
		if err != nil {
			return err
		}
		transaction, err := connection.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx, string(migration)); err != nil {
			_ = transaction.Rollback(ctx)
			return errors.New("migration " + entry.Name() + ": " + err.Error())
		}
		if _, err := transaction.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, entry.Name()); err != nil {
			_ = transaction.Rollback(ctx)
			return err
		}
		if err := transaction.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *postgresRepository) seed(ctx context.Context) error {
	seedUsers := []struct {
		id, name, email, handle, initials, color string
		isBot                                    bool
	}{
		{"u-naoki", "Naoki Sato", "demo@example.com", "naoki", "NS", "linear-gradient(135deg, #f3a683, #c56cf0)", false},
		{"u-ayaka", "Ayaka Mori", "ayaka@example.com", "ayaka", "AM", "linear-gradient(135deg, #f8c291, #e55039)", false},
		{"u-ken", "Ken Ito", "ken@example.com", "ken", "KI", "linear-gradient(135deg, #82ccdd, #60a3bc)", false},
		{orbitAIUserID, "Orbit AI", "orbit-ai@local", "orbit-ai", "✦", "linear-gradient(135deg, #8b5cf6, #22d3ee)", true},
	}
	for _, seedUser := range seedUsers {
		hash, err := hashPassword("demo-password")
		if err != nil {
			return err
		}
		if _, err := r.pool.Exec(ctx, `INSERT INTO users (id, name, email, handle, initials, color, is_bot, password_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO UPDATE SET is_bot=excluded.is_bot`, seedUser.id, seedUser.name, seedUser.email, seedUser.handle, seedUser.initials, seedUser.color, seedUser.isBot, hash); err != nil {
			return err
		}
	}

	for position, channel := range seededChannels() {
		if _, err := r.pool.Exec(ctx, `INSERT INTO channels (id, name, channel_group, kind, description, presence, initials, color, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (id) DO NOTHING`, channel.ID, channel.Name, channel.Group, channel.Kind, channel.Description, nullableString(channel.Presence), nullableString(channel.Initials), nullableString(channel.Color), time.Now().Add(time.Duration(position)*time.Millisecond)); err != nil {
			return err
		}
	}
	addMembership := func(channelID, userID, role string) error {
		_, err := r.pool.Exec(ctx, `INSERT INTO channel_members (channel_id,user_id,role) VALUES ($1,$2,$3) ON CONFLICT (channel_id,user_id) DO NOTHING`, channelID, userID, role)
		return err
	}
	for _, channel := range seededChannels() {
		if channel.Kind == "channel" {
			for _, userID := range []string{"u-naoki", "u-ayaka", "u-ken", orbitAIUserID} {
				if err := addMembership(channel.ID, userID, "member"); err != nil {
					return err
				}
			}
		}
	}
	for _, userID := range []string{"u-naoki", "u-ayaka", orbitAIUserID} {
		if err := addMembership("ayaka", userID, "member"); err != nil {
			return err
		}
	}
	for _, userID := range []string{"u-naoki", "u-ken", orbitAIUserID} {
		if err := addMembership("ken", userID, "member"); err != nil {
			return err
		}
	}
	for _, userID := range []string{"u-naoki", orbitAIUserID} {
		if err := addMembership("orbit-ai", userID, "member"); err != nil {
			return err
		}
	}

	var hasMessages bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages)`).Scan(&hasMessages); err != nil {
		return err
	}
	if !hasMessages {
		seedMessages := []struct {
			id, channelID, authorID, body string
			reactions                     string
			threadCount                   int
			createdAt                     time.Time
		}{
			{"g-1", "general", "u-ken", "おはようございます。今週もよろしくお願いします！", `[{"emoji":"☀️","count":5}]`, 0, time.Now().Add(-5 * time.Hour)},
			{"g-2", "general", "u-naoki", "おはよう！リアルタイムチャットの初期画面を作り始めます。", `[{"emoji":"🚀","count":2}]`, 0, time.Now().Add(-4*time.Hour - 57*time.Minute)},
			{"f-1", "frontend", "u-ayaka", "APIレスポンスの型定義、shared/typesに置いておくと使いやすそうです。", `[{"emoji":"👍","count":3}]`, 0, time.Now().Add(-24 * time.Hour)},
			{"ds-1", "design-system", "u-ayaka", "新しいカラートークンをまとめました。", `[{"emoji":"✨","count":4}]`, 3, time.Now().Add(-3 * time.Hour)},
		}
		for _, message := range seedMessages {
			if _, err := r.pool.Exec(ctx, `INSERT INTO messages (id, channel_id, author_id, body, reactions, thread_count, created_at) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7) ON CONFLICT (id) DO NOTHING`, message.id, message.channelID, message.authorID, message.body, message.reactions, message.threadCount, message.createdAt); err != nil {
				return err
			}
		}
	}
	if err := r.backfillMessageSequences(ctx); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO channel_read_states (user_id, channel_id, last_read_sequence, last_read_message_id) SELECT 'u-naoki', channel_id, max(created_sequence), (array_agg(id ORDER BY created_sequence DESC))[1] FROM messages GROUP BY channel_id ON CONFLICT (user_id, channel_id) DO NOTHING`)
	return err
}

func (r *postgresRepository) backfillMessageSequences(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `SELECT id FROM messages WHERE created_sequence=0 ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		transaction, err := r.pool.Begin(ctx)
		if err != nil {
			return err
		}
		message, err := r.getMessageFrom(ctx, transaction, id)
		if err != nil {
			_ = transaction.Rollback(ctx)
			return err
		}
		record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "message.created", ChannelID: message.ChannelID, Message: pointerToMessage(message)})
		if err != nil {
			_ = transaction.Rollback(ctx)
			return err
		}
		if _, err := transaction.Exec(ctx, `UPDATE messages SET created_sequence=$1 WHERE id=$2`, record.Sequence, id); err != nil {
			_ = transaction.Rollback(ctx)
			return err
		}
		if err := transaction.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
