package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

func TestDownloadTimingPauseRetryAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	db.now = func() time.Time { return now }
	snapshot := engine.Task{ID: "timed", Engine: "http", CreatedAt: now, UpdatedAt: now}
	save := func(status engine.TaskStatus, want int64) {
		t.Helper()
		// Every observation is a fresh engine snapshot, without application timing.
		snapshot.Status = status
		got, err := db.Save(ctx, snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if got.ElapsedMS != want {
			t.Fatalf("%s elapsed=%d want=%d", status, got.ElapsedMS, want)
		}
	}
	save(engine.StatusQueued, 0)
	now = now.Add(time.Hour)
	save(engine.StatusRunning, 0)
	now = now.Add(2500 * time.Millisecond)
	save(engine.StatusRunning, 2500)
	now = now.Add(1500 * time.Millisecond)
	save(engine.StatusPaused, 4000)
	now = now.Add(2 * time.Hour)
	save(engine.StatusQueued, 4000)
	now = now.Add(time.Hour)
	save(engine.StatusRunning, 4000)
	now = now.Add(3 * time.Second)
	save(engine.StatusFailed, 7000)
	now = now.Add(time.Minute)
	save(engine.StatusRunning, 7000)
	now = now.Add(time.Second)
	save(engine.StatusRunning, 8000)
	// Simulate a process exit while the persisted task still says running.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	db.now = func() time.Time { return now }
	save(engine.StatusPaused, 8000)
	save(engine.StatusRunning, 8000)
	now = now.Add(2 * time.Second)
	save(engine.StatusCompleted, 10000)
	now = now.Add(time.Hour)
	save(engine.StatusCompleted, 10000)
	records, err := db.List(ctx)
	if err != nil || len(records) != 1 || records[0].ElapsedMS != 10000 {
		t.Fatalf("persistent timing: %#v %v", records, err)
	}
	if err := db.Delete(ctx, snapshot.ID); err != nil {
		t.Fatal(err)
	}
	save(engine.StatusRunning, 0)
}

func TestDownloadTimingWithoutBrowserPolling(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now()
	db.now = func() time.Time { return now }
	task := engine.Task{ID: "background", Engine: "aria2", Status: engine.StatusRunning}
	if err := db.Upsert(ctx, task); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	// The background queue records the transition even with no browser connected.
	task.Status = engine.StatusCompleted
	if err := db.Upsert(ctx, task); err != nil {
		t.Fatal(err)
	}
	items, err := db.List(ctx)
	if err != nil || len(items) != 1 || items[0].ElapsedMS != 10000 {
		t.Fatalf("background timing: %#v %v", items, err)
	}
}
