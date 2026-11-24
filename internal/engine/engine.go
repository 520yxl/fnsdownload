package engine

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("engine task not found")
	ErrUnsupported = errors.New("operation unsupported by engine")
)

type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusPaused    TaskStatus = "paused"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusRemoved   TaskStatus = "removed"
)

type Task struct {
	Retryable      bool       `json:"retryable,omitempty"`
	RetryCount     int        `json:"retry_count,omitempty"`
	RetryAt        *time.Time `json:"retry_at,omitempty"`
	ID             string     `json:"id"`
	Engine         string     `json:"engine"`
	Source         string     `json:"source"`
	Name           string     `json:"name"`
	Directory      string     `json:"directory"`
	Status         TaskStatus `json:"status"`
	TotalBytes     int64      `json:"total_bytes"`
	CompletedBytes int64      `json:"completed_bytes"`
	SpeedBytes     int64      `json:"speed_bytes"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateRequest struct {
	URL       string `json:"url"`
	Directory string `json:"directory"`
	Name      string `json:"name,omitempty"`
}

type Engine interface {
	Name() string
	Initialize(context.Context) error
	Health(context.Context) error
	Create(context.Context, CreateRequest) (Task, error)
	Start(context.Context, string) error
	Pause(context.Context, string) error
	Resume(context.Context, string) error
	Delete(context.Context, string, bool) error
	Get(context.Context, string) (Task, error)
	List(context.Context) ([]Task, error)
	Close() error
}
