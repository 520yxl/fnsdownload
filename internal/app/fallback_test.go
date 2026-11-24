package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
	httpengine "github.com/520yxl/fnsdownload/internal/engine/http"
	"github.com/520yxl/fnsdownload/internal/store"
)

type fallbackTestEngine struct {
	*queueTestEngine
	creates             int
	createErr, pauseErr error
}

func (e *fallbackTestEngine) Health(context.Context) error { return nil }
func (e *fallbackTestEngine) Create(_ context.Context, r engine.CreateRequest) (engine.Task, error) {
	e.creates++
	if e.createErr != nil {
		return engine.Task{}, e.createErr
	}
	task := engine.Task{ID: e.name + "-native", Engine: e.name, Source: r.URL, Name: r.Name, Directory: r.Directory, Status: engine.StatusPaused, CreatedAt: time.Now()}
	e.tasks[task.ID] = task
	return task, nil
}
func (e *fallbackTestEngine) List(context.Context) ([]engine.Task, error) {
	var tasks []engine.Task
	for _, task := range e.tasks {
		tasks = append(tasks, task)
	}
	return tasks, nil
}
func (e *fallbackTestEngine) Pause(ctx context.Context, id string) error {
	if e.pauseErr != nil {
		return e.pauseErr
	}
	return e.queueTestEngine.Pause(ctx, id)
}
func (e *fallbackTestEngine) Delete(_ context.Context, id string, files bool) error {
	if files {
		return errors.New("fallback must preserve files")
	}
	if _, ok := e.tasks[id]; !ok {
		return engine.ErrNotFound
	}
	delete(e.tasks, id)
	return nil
}
func fallbackFixture(t *testing.T) (*Handler, *fallbackTestEngine, *fallbackTestEngine, *fallbackTestEngine, engine.Task) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	manager := engine.NewManager()
	makeEngine := func(name string) *fallbackTestEngine {
		e := &fallbackTestEngine{queueTestEngine: &queueTestEngine{name: name, tasks: map[string]engine.Task{}}}
		manager.Register(e)
		return e
	}
	x, a, h := makeEngine("xunlei"), makeEngine("aria2"), makeEngine("http")
	task := engine.Task{ID: "stable", Engine: "xunlei", Source: "https://example.com/file.zip", Name: "file.zip", Directory: root, FallbackRoot: root, AutoEngine: true, TriedEngines: []string{"xunlei"}, Status: engine.StatusFailed, Error: "迅雷任务错误 111176，授权状态 0", CreatedAt: time.Now().Add(-time.Hour), ElapsedMS: 42000}
	x.tasks[task.ID] = task
	if err := db.Upsert(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{cfg: Config{DataDir: root, DownloadDir: root}, store: db, engines: manager, spaceProbe: func(string) (volumeSpace, error) { return volumeSpace{Available: 1 << 40, Volume: "test"}, nil }}
	return handler, x, a, h, task
}
func TestFallbackKeepsIdentityQueueAndFileIsolation(t *testing.T) {
	ctx := context.Background()
	h, x, a, http, original := fallbackFixture(t)
	partial := filepath.Join(original.Directory, "file.zip.part")
	if err := os.WriteFile(partial, []byte("original partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if !h.shouldFallback(original, time.Now()) {
		t.Fatal("documented SDK timeout did not qualify")
	}
	switched, ok := h.fallbackTask(ctx, original)
	if !ok || switched.ID != original.ID || switched.Engine != "aria2" || switched.NativeID != "aria2-native" || !switched.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("lost stable task identity: %#v", switched)
	}
	if switched.Directory == original.Directory || filepath.Dir(switched.Directory) != original.Directory {
		t.Fatal("fallback reused original partial path")
	}
	if _, err := x.Get(ctx, original.ID); err == nil {
		t.Fatal("old native task retained")
	}
	if len(a.starts) != 0 || len(http.starts) != 0 {
		t.Fatal("fallback bypassed queue")
	}
	if len(h.queue) != 1 || switched.Status != engine.StatusQueued {
		t.Fatal("fallback not queued")
	}
	records, _ := h.store.List(ctx)
	if len(records) != 1 || records[0].ElapsedMS != 42000 || !slices.Contains(records[0].RetiredNativeIDs, "xunlei/stable") {
		t.Fatalf("history split or elapsed lost: %#v", records)
	}
	// The shared scheduler starts the new native task using the stable ID binding.
	h.dispatchQueue(ctx)
	if len(a.starts) != 1 || a.starts[0] != "aria2-native" {
		t.Fatal("native ID not routed through queue", a.starts)
	}
	r := httptest.NewRequest("POST", "/api/tasks/stable/pause", nil)
	r.Header.Set("X-Trim-Userid", "1000")
	r.SetPathValue("id", "stable")
	r.SetPathValue("action", "pause")
	w := httptest.NewRecorder()
	h.taskAction(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	raw, _ := a.Get(ctx, "aria2-native")
	if raw.Status != engine.StatusPaused {
		t.Fatal("stable ID did not pause native transfer")
	}
	content, err := os.ReadFile(partial)
	if err != nil || string(content) != "original partial" {
		t.Fatal("original partial changed")
	}
	// Retry after an aria2 network failure moves to HTTP, never back to Xunlei.
	delete(h.retryBlocked, original.ID)
	raw.Status = engine.StatusFailed
	raw.Error = "aria2 任务错误 2"
	a.tasks[raw.ID] = raw
	saved, _ := h.store.Get(ctx, original.ID)
	bound, _ := h.engineFor(saved)
	failed, _ := bound.Get(ctx, original.ID)
	last, ok := h.fallbackTask(ctx, failed)
	if !ok || last.Engine != "http" {
		t.Fatalf("next engine not selected: %#v", last)
	}
	if !slices.Contains(last.RetiredNativeIDs, "aria2/aria2-native") || !slices.Contains(last.RetiredNativeIDs, "xunlei/stable") {
		t.Fatal("retired native bindings lost", last.RetiredNativeIDs)
	}
	last.Status = engine.StatusFailed
	last.Error = "HTTP status 503"
	if _, ok := h.fallbackTask(ctx, last); ok {
		t.Fatal("engine fallback looped")
	}
	// Old/unbound SDK records cannot create duplicate list rows.
	x.tasks["stale"] = engine.Task{ID: "stale", Engine: "xunlei", Status: engine.StatusFailed}
	r = httptest.NewRequest("GET", "/api/tasks", nil)
	r.Header.Set("X-Trim-Userid", "1000")
	w = httptest.NewRecorder()
	h.listTasks(w, r)
	var listed []engine.Task
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "stable" || listed[0].Engine != "http" {
		t.Fatalf("unstable list: %s", w.Body.String())
	}
}
func TestFallbackStallProgressAndSafety(t *testing.T) {
	h, x, _, _, task := fallbackFixture(t)
	now := time.Now()
	task.Status = engine.StatusRunning
	task.Error = ""
	if h.shouldFallback(task, now) || h.shouldFallback(task, now.Add(119*time.Second)) {
		t.Fatal("switched too early")
	}
	task.CompletedBytes = 1
	if h.shouldFallback(task, now.Add(120*time.Second)) || h.shouldFallback(task, now.Add(239*time.Second)) {
		t.Fatal("progress did not reset stall clock")
	}
	if !h.shouldFallback(task, now.Add(240*time.Second)) {
		t.Fatal("stalled task not eligible")
	}
	x.tasks[task.ID] = task
	x.pauseErr = errors.New("cannot stop")
	if _, ok := h.fallbackTask(context.Background(), task); ok {
		t.Fatal("switched without stopping old transfer")
	}
	for _, change := range []func(*engine.Task){func(t *engine.Task) { t.AutoEngine = false }, func(t *engine.Task) { t.HeadersRequired = true }, func(t *engine.Task) { t.Status = engine.StatusPaused }, func(t *engine.Task) { t.Status = engine.StatusQueued }, func(t *engine.Task) { t.Status = engine.StatusCompleted }} {
		candidate := task
		change(&candidate)
		if h.shouldFallback(candidate, now.Add(time.Hour)) {
			t.Fatal("unsafe task qualified", candidate)
		}
	}
}
func TestFallbackErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		engine, message string
		want            bool
	}{
		{"xunlei", "迅雷任务错误 111176，授权状态 0（长时间未收到数据）", true},
		{"xunlei", "迅雷任务错误 119212，授权状态 0", true},
		{"xunlei", "迅雷任务错误 111176，授权状态 1", false},
		{"xunlei", "迅雷任务错误 111085，授权状态 0", false},
		{"xunlei", "迅雷任务错误 111128，授权状态 0", false},
		{"xunlei", "迅雷任务错误 111154，授权状态 0", false},
		{"http", "HTTP status 403", false}, {"http", "HTTP status 404", false},
		{"http", "no space left on device", false}, {"http", "permission denied", false},
		{"http", "certificate verify failed", false}, {"http", "HTTP status 503", true},
		{"aria2", "aria2 任务错误 9", false}, {"aria2", "aria2 任务错误 2", true},
	} {
		if got := transientDownloadFailure(engine.Task{Engine: tc.engine, Status: engine.StatusFailed, Error: tc.message}); got != tc.want {
			t.Fatalf("%s %q: %v", tc.engine, tc.message, got)
		}
	}
}
func TestFallbackBindingSurvivesRestore(t *testing.T) {
	h, _, _, _, original := fallbackFixture(t)
	// Use the real HTTP restore path, with a logical ID from a different engine.
	native := httpengine.New()
	h.httpEngine = native
	replacement, _ := native.Create(context.Background(), engine.CreateRequest{URL: original.Source, Directory: original.Directory, Name: original.Name})
	original.Engine = "http"
	original.NativeID = replacement.ID
	original.Status = engine.StatusRunning
	if err := h.store.Upsert(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	manager := engine.NewManager()
	manager.Register(native)
	h.engines = manager
	// Reopen SQLite to simulate a fresh application process (no timing anchors).
	path := filepath.Join(h.cfg.DataDir, "tasks.db")
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	h.store = reopened
	if err := h.restoreTasks(); err != nil {
		t.Fatal(err)
	}
	saved, _ := h.store.Get(context.Background(), original.ID)
	if saved.ID != original.ID || saved.NativeID != replacement.ID || saved.ElapsedMS != 42000 || saved.Status != engine.StatusPaused {
		t.Fatalf("restore lost binding: %#v", saved)
	}
	bound, _ := h.engineFor(saved)
	restored, err := bound.Get(context.Background(), original.ID)
	if err != nil || restored.ID != original.ID {
		t.Fatal("restored logical ID unusable", restored, err)
	}
}

func TestAutomaticFallbackCompletesRealHTTPDownload(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("verified fallback payload")) }))
	defer server.Close()
	h, x, _, _, task := fallbackFixture(t)
	direct := httpengine.New()
	defer direct.Close()
	manager := engine.NewManager()
	manager.Register(x)
	manager.Register(direct)
	h.engines = manager
	task.Source = server.URL + "/file.zip"
	if err := h.store.Upsert(ctx, task); err != nil {
		t.Fatal(err)
	}
	x.tasks[task.ID] = task
	// No browser/API polling: only the ordinary backend scheduler drives fallback.
	h.dispatchQueue(ctx)
	saved, err := h.store.Get(ctx, task.ID)
	if err != nil || saved.Engine != "http" || saved.NativeID == "" {
		t.Fatalf("fallback did not bind real HTTP task: %#v %v", saved, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.dispatchQueue(ctx)
		saved, err = h.store.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Status == engine.StatusCompleted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if saved.Status != engine.StatusCompleted {
		t.Fatalf("fallback did not finish: %#v", saved)
	}
	content, err := os.ReadFile(filepath.Join(saved.Directory, saved.Name))
	if err != nil || string(content) != "verified fallback payload" {
		t.Fatal("fallback file integrity", string(content), err)
	}
	records, _ := h.store.List(ctx)
	if len(records) != 1 || records[0].ID != task.ID || records[0].ElapsedMS < 42000 {
		t.Fatalf("fallback split history: %#v", records)
	}
}
