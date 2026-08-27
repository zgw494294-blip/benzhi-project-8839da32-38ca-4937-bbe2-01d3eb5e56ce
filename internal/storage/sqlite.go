package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"stage-rigging-safety-release/internal/domain"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db            *sql.DB
	now           func() time.Time
	timelineCache map[string][]domain.AuditEvent
}

func Open(path string) (*SQLiteStore, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	s := &SQLiteStore{db: db, now: func() time.Time { return time.Now().UTC() }, timelineCache: map[string][]domain.AuditEvent{}}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS campaigns (id TEXT PRIMARY KEY, version INTEGER NOT NULL, status TEXT NOT NULL, data BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS audit_events (sequence INTEGER PRIMARY KEY AUTOINCREMENT, campaign_id TEXT NOT NULL, action TEXT NOT NULL, actor TEXT NOT NULL, role TEXT NOT NULL, version INTEGER NOT NULL, accepted INTEGER NOT NULL, reason TEXT NOT NULL, details BLOB, occurred_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS audit_campaign_sequence ON audit_events(campaign_id, sequence)`,
		`CREATE TABLE IF NOT EXISTS idempotency (campaign_id TEXT NOT NULL, idem_key TEXT NOT NULL, operation TEXT NOT NULL, response BLOB NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(campaign_id, idem_key))`,
		`CREATE TABLE IF NOT EXISTS permits (permit_number TEXT PRIMARY KEY, campaign_id TEXT NOT NULL UNIQUE, frozen_digest TEXT NOT NULL, issued_at TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, datetime('now'))`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("执行数据库迁移: %w", err)
		}
	}
	return nil
}
