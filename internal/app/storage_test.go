package app

import (
	"context"
	"errors"
	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/store"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func storageTestHandler(t *testing.T) (*Handler, *queueTestEngine) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &queueTestEngine{name: "http", tasks: map[string]engine.Task{}}
	m := engine.NewManager()
	m.Register(e)
	return &Handler{cfg: Config{DataDir: dir}, store: db, engines: m}, e
}
func TestDiskReservationsSharedByVolume(t *testing.T) {
	ctx := context.Background()
	h, e := storageTestHandler(t)
	free := int64(1500 << 20)
	h.spaceProbe = func(path string) (volumeSpace, error) {
		vol := "shared"
		if strings.HasPrefix(path, "other") {
			vol = "other"
		}
		return volumeSpace{Available: free, Volume: vol}, nil
	}
	for i, id := range []string{"first", "second", "other"} {
		task := engine.Task{ID: id, Engine: "http", Directory: id, Status: engine.StatusPaused, TotalBytes: 600 << 20, CreatedAt: time.Unix(int64(i), 0)}
		e.tasks[id] = task
		if err := h.enqueue(ctx, e, id); err != nil {
			t.Fatal(err)
		}
	}
	h.dispatchQueue(ctx)
	if len(e.starts) != 2 || e.starts[0] != "first" || e.starts[1] != "other" || h.spaceWait["second"] == "" {
		t.Fatal("same-volume reservations or independent volume incorrect", e.starts, h.spaceWait)
	}
	task := e.tasks["first"]
	task.Status = engine.StatusCompleted
	e.tasks["first"] = task
	h.dispatchQueue(ctx)
	if len(e.starts) != 3 || e.starts[2] != "second" || h.spaceWait["second"] != "" {
		t.Fatal("waiting task did not start when space became available")
	}
}
func TestDiskLowSpacePausesOwnedTaskWithoutAutoResume(t *testing.T) {
	ctx := context.Background()
	h, e := storageTestHandler(t)
	free := int64(256 << 20)
	h.spaceProbe = func(string) (volumeSpace, error) { return volumeSpace{Available: free, Volume: "shared"}, nil }
	own := engine.Task{ID: "own", Engine: "http", Status: engine.StatusRunning, Directory: "own"}
	e.tasks[own.ID] = own
	// This represents a task the engine knows about but the application did not register.
	e.tasks["foreign"] = engine.Task{ID: "foreign", Engine: "http", Status: engine.StatusRunning, Directory: "foreign"}
	if err := h.store.Upsert(ctx, own); err != nil {
		t.Fatal(err)
	}
	h.dispatchQueue(ctx)
	if e.tasks["own"].Status != engine.StatusPaused || h.spacePaused["own"] == "" || !h.retryBlocked["own"] {
		t.Fatal("low-space task was not paused")
	}
	if e.tasks["foreign"].Status != engine.StatusRunning {
		t.Fatal("foreign task touched")
	}
	free = 10 << 30
	h.dispatchQueue(ctx)
	if len(e.starts) != 0 {
		t.Fatal("space-paused task resumed automatically")
	}
	if err := h.enqueue(ctx, e, "own"); err != nil {
		t.Fatal(err)
	}
	h.dispatchQueue(ctx)
	if len(e.starts) != 1 || h.spacePaused["own"] != "" {
		t.Fatal("manual resume did not clear space warning")
	}
}
func TestUnknownSizeAndUnavailableStorage(t *testing.T) {
	ctx := context.Background()
	h, e := storageTestHandler(t)
	task := engine.Task{ID: "unknown", Engine: "http", Directory: "folder", Status: engine.StatusPaused}
	e.tasks[task.ID] = task
	h.spaceProbe = func(string) (volumeSpace, error) { return volumeSpace{}, errors.New("unmounted") }
	h.enqueue(ctx, e, task.ID)
	h.dispatchQueue(ctx)
	if len(e.starts) != 0 || h.spaceWait[task.ID] == "" {
		t.Fatal("unreadable storage treated as sufficient")
	}
	h.spaceProbe = func(string) (volumeSpace, error) { return volumeSpace{Available: (512 << 20) - 1, Volume: "one"}, nil }
	h.dispatchQueue(ctx)
	if len(e.starts) != 0 {
		t.Fatal("unknown length bypassed reserve")
	}
	h.spaceProbe = func(string) (volumeSpace, error) { return volumeSpace{Available: (512 << 20) + 1, Volume: "one"}, nil }
	h.dispatchQueue(ctx)
	if len(e.starts) != 1 {
		t.Fatal("unknown length prevented safe start")
	}
	if spaceRequired(math.MaxInt64-1, 100) != math.MaxInt64 {
		t.Fatal("space arithmetic overflow")
	}
}
func TestNativeVolumeSpace(t *testing.T) {
	dir := t.TempDir()
	a, err := readVolumeSpace(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := readVolumeSpace(filepath.Join(dir, "."))
	if err != nil || a.Volume == "" || a.Volume != b.Volume || a.Available <= 0 {
		t.Fatal("invalid native volume information", a, b, err)
	}
}
