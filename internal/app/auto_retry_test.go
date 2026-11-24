package app

import (
	"context"
	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/store"
	"path/filepath"
	"testing"
	"time"
)

func TestAutomaticRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		engine, message string
		want            bool
	}{
		{"http", "HTTP status 503 Service Unavailable", true},
		{"http", "HTTP status 429 Too Many Requests", true},
		{"http", "connection reset by peer", true},
		{"http", "HTTP status 403 Forbidden", false},
		{"http", "HTTP status 404 Not Found", false},
		{"http", "certificate has expired", false},
		{"http", "write: no space left on device", false},
		{"http", "permission denied: timeout", false},
		{"aria2", "aria2 任务错误 2", true},
		{"aria2", "aria2 任务错误 29", true},
		{"aria2", "aria2 任务错误 24", false},
		{"aria2", "aria2 任务错误 9", false},
		{"aria2", "aria2 任务错误 22", false},
		{"xunlei", "迅雷任务错误 1，授权状态 1", false},
	} {
		if got := transientDownloadFailure(engine.Task{Engine: tc.engine, Error: tc.message, Status: engine.StatusFailed}); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestAutomaticRetryDelayBudgetAndCancellation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := &queueTestEngine{name: "http", tasks: map[string]engine.Task{"job": {ID: "job", Engine: "http", Status: engine.StatusFailed, Error: "HTTP status 503 Service Unavailable"}}}
	m := engine.NewManager()
	m.Register(e)
	h := &Handler{cfg: Config{DataDir: dir}, store: db, engines: m}
	task := e.tasks["job"]
	if err := db.Upsert(ctx, task); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now)
	state := h.retryStates[task.ID]
	if state.Next == nil || !state.Next.Equal(now.Add(30*time.Second)) || len(h.queue) != 0 {
		t.Fatal("missing initial delay")
	}
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now.Add(29*time.Second))
	if len(h.queue) != 0 {
		t.Fatal("retry started early")
	}
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now.Add(30*time.Second))
	if len(h.queue) != 1 || h.retryStates[task.ID].Count != 1 || len(e.starts) != 0 {
		t.Fatal("retry bypassed queue or budget")
	}
	delete(h.queue, task.ID) // simulate a completed attempt that failed again
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now.Add(31*time.Second))
	if !h.retryStates[task.ID].Next.Equal(now.Add(91 * time.Second)) {
		t.Fatal("delay did not increase")
	}
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now.Add(91*time.Second))
	delete(h.queue, task.ID)
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now.Add(1000*time.Second))
	if h.retryStates[task.ID].Count != 2 || h.retryStates[task.ID].Next != nil || len(h.queue) != 0 {
		t.Fatal("retry limit exceeded")
	}
	h.retryStates = nil
	h.retryBlocked = map[string]bool{task.ID: true}
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now)
	if len(h.retryStates) != 0 || h.retryMetadata(task).Retryable {
		t.Fatal("cancelled or restored task scheduled")
	}
	h.retryBlocked = nil
	h.prepareAutoRetries(ctx, []engine.Task{task}, 2, now)
	h.prepareAutoRetries(ctx, []engine.Task{task}, 0, now.Add(time.Hour))
	if len(h.retryStates) != 0 || len(h.queue) != 0 {
		t.Fatal("disabled retry continued")
	}
}
