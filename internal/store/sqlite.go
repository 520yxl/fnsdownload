package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
	_ "modernc.org/sqlite"
)

type Store struct {
	db       *sql.DB
	timingMu sync.Mutex
	active   map[string]time.Time
	now      func() time.Time
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, active: make(map[string]time.Time), now: time.Now}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite (%s): %w", pragma, err)
		}
	}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS download_tasks (
		id TEXT PRIMARY KEY NOT NULL,
		engine TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		payload BLOB NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("migrate download_tasks: %w", err)
	}
	_, err = s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_download_tasks_status_updated ON download_tasks(status, updated_at DESC)")
	if err != nil {
		return fmt.Errorf("index download_tasks: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY NOT NULL,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	_, err = s.db.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record schema migration: %w", err)
	}
	return nil
}

// Save preserves application-owned timing when engines return fresh task snapshots.
// Active checkpoints exist only for this process, so application downtime is never counted.
func (s *Store) Save(ctx context.Context, task engine.Task) (engine.Task, error) {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()
	if task.ID == "" || task.Engine == "" {
		return task, errors.New("task id and engine are required")
	}
	var previous []byte
	err := s.db.QueryRowContext(ctx, "SELECT payload FROM download_tasks WHERE id = ?", task.ID).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return task, fmt.Errorf("read task timing: %w", err)
	}
	if err == nil {
		var saved engine.Task
		if err := json.Unmarshal(previous, &saved); err != nil {
			return task, fmt.Errorf("read task timing: %w", err)
		}
		task.ElapsedMS = saved.ElapsedMS
	}
	if task.ElapsedMS < 0 {
		task.ElapsedMS = 0
	}
	now := s.now()
	if checkpoint, running := s.active[task.ID]; running {
		if delta := now.Sub(checkpoint).Milliseconds(); delta > 0 {
			task.ElapsedMS += delta
		}
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return task, fmt.Errorf("encode task: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO download_tasks(id, engine, status, created_at, updated_at, payload)
  VALUES(?, ?, ?, ?, ?, ?)
  ON CONFLICT(id) DO UPDATE SET engine=excluded.engine, status=excluded.status,
  updated_at=excluded.updated_at, payload=excluded.payload`,
		task.ID, task.Engine, string(task.Status), task.CreatedAt.UTC().Format(time.RFC3339Nano),
		task.UpdatedAt.UTC().Format(time.RFC3339Nano), payload)
	if err != nil {
		return task, fmt.Errorf("save task %s: %w", task.ID, err)
	}
	if task.Status == engine.StatusRunning {
		s.active[task.ID] = now
	} else {
		delete(s.active, task.ID)
	}
	return task, nil
}

func (s *Store) Upsert(ctx context.Context, task engine.Task) error {
	_, err := s.Save(ctx, task)
	return err
}

func (s *Store) List(ctx context.Context) ([]engine.Task, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM download_tasks ORDER BY updated_at DESC")
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	out := make([]engine.Task, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var task engine.Task
		if err := json.Unmarshal(payload, &task); err != nil {
			return nil, fmt.Errorf("decode stored task: %w", err)
		}
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()
	_, err := s.db.ExecContext(ctx, "DELETE FROM download_tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}
	delete(s.active, id)
	return nil
}

func (s *Store) Close() error { return s.db.Close() }
