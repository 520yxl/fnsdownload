package app

import (
	"context"
	"errors"
	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type queueTestEngine struct {
	engine.Engine
	name   string
	tasks  map[string]engine.Task
	starts []string
	fail   bool
}

func (e *queueTestEngine) Name() string { return e.name }
func (e *queueTestEngine) Get(_ context.Context, id string) (engine.Task, error) {
	t, ok := e.tasks[id]
	if !ok {
		return t, engine.ErrNotFound
	}
	return t, nil
}
func (e *queueTestEngine) Start(_ context.Context, id string) error {
	if e.fail {
		return errors.New("start failed")
	}
	t := e.tasks[id]
	t.Status = engine.StatusRunning
	e.tasks[id] = t
	e.starts = append(e.starts, id)
	return nil
}

func (e *queueTestEngine) Pause(_ context.Context, id string) error {
	t, ok := e.tasks[id]
	if !ok {
		return engine.ErrNotFound
	}
	t.Status = engine.StatusPaused
	t.SpeedBytes = 0
	e.tasks[id] = t
	return nil
}

func TestGlobalQueueAcrossEngines(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"interval":2500,"theme":"auto","max_concurrent":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := &queueTestEngine{name: "first", tasks: map[string]engine.Task{}}
	b := &queueTestEngine{name: "second", tasks: map[string]engine.Task{}}
	manager := engine.NewManager()
	manager.Register(a)
	manager.Register(b)
	h := &Handler{cfg: Config{DataDir: dir}, store: db, engines: manager, spaceProbe: func(string) (volumeSpace, error) { return volumeSpace{Available: 1 << 40, Volume: "test"}, nil }}
	now := time.Now()
	for i, e := range []*queueTestEngine{a, b} {
		id := e.name
		task := engine.Task{ID: id, Engine: id, Status: engine.StatusPaused, CreatedAt: now.Add(time.Duration(i) * time.Second)}
		e.tasks[id] = task
		if err := h.enqueue(ctx, e, id); err != nil {
			t.Fatal(err)
		}
	}
	h.dispatchQueue(ctx)
	if len(a.starts) != 1 || len(b.starts) != 0 {
		t.Fatal("global limit did not span engines")
	}
	// Enqueuing an already active task must not reserve another queue slot.
	if err := h.enqueue(ctx, a, "first"); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.queue["first"]; ok {
		t.Fatal("active task enqueued twice")
	}
	task := a.tasks["first"]
	task.Status = engine.StatusCompleted
	a.tasks["first"] = task
	h.dispatchQueue(ctx)
	if len(b.starts) != 1 {
		t.Fatal("completion did not release slot")
	}
	// Failed starts must leave the queue and remain visible as failures.
	task = b.tasks["second"]
	task.Status = engine.StatusPaused
	b.tasks["second"] = task
	b.fail = true
	h.enqueue(ctx, b, "second")
	h.dispatchQueue(ctx)
	if len(h.queue) != 0 || h.queueFailures["second"] == "" {
		t.Fatal("failed start stuck in queue or lost error")
	}
}

func TestQueueFailureDoesNotOccupySlot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"interval":2500,"theme":"auto","max_concurrent":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := &queueTestEngine{name: "http", tasks: map[string]engine.Task{}, fail: true}
	manager := engine.NewManager()
	manager.Register(a)
	h := &Handler{cfg: Config{DataDir: dir}, store: db, engines: manager, spaceProbe: func(string) (volumeSpace, error) { return volumeSpace{Available: 1 << 40, Volume: "test"}, nil }}
	for _, id := range []string{"bad", "good"} {
		a.tasks[id] = engine.Task{ID: id, Engine: "http", Status: engine.StatusQueued, CreatedAt: time.Now()}
		if err := h.enqueue(ctx, a, id); err != nil {
			t.Fatal(err)
		}
		h.dispatchQueue(ctx)
		a.fail = false
	}
	if len(a.starts) != 1 || a.starts[0] != "good" {
		t.Fatal("failed task occupied a download slot")
	}
}
