package app

import (
	"context"
	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDuplicateLinksAndExplicitOverride(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Upsert(ctx, engine.Task{ID: "original", Engine: "http", Source: "https://EXAMPLE.com:443/file.zip#one", Status: engine.StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: db, engines: engine.NewManager()}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"url":"https://example.com/file.zip#two","engine":"missing"}`, 409},
		{`{"url":"https://example.com/file.zip","engine":"missing","allow_duplicate":true}`, 400},
		{`{"url":"https://example.com/file.zip?signature=new","engine":"missing"}`, 400},
	} {
		r := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(tc.body))
		r.Header.Set("X-Trim-Userid", "1000")
		w := httptest.NewRecorder()
		h.createTask(w, r)
		if w.Code != tc.status {
			t.Fatal(tc.body, w.Code, w.Body.String())
		}
	}
	items, _ := db.List(ctx)
	if len(items) != 1 {
		t.Fatal("duplicate guard changed tasks")
	}
	if canonicalDownloadURL("https://example.com/File.zip") == canonicalDownloadURL("https://example.com/file.zip") {
		t.Fatal("case-sensitive paths merged")
	}
}
func TestRetryUsesExistingTaskAndQueue(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := &queueTestEngine{name: "http", tasks: map[string]engine.Task{"failed": {ID: "failed", Engine: "http", Status: engine.StatusFailed, Error: "old error"}, "done": {ID: "done", Engine: "http", Status: engine.StatusCompleted}}}
	for _, task := range e.tasks {
		if err := db.Upsert(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	m := engine.NewManager()
	m.Register(e)
	h := &Handler{store: db, engines: m}
	for _, tc := range []struct {
		id     string
		status int
	}{{"failed", 204}, {"done", 409}} {
		r := httptest.NewRequest("POST", "/api/tasks/"+tc.id+"/retry", nil)
		r.Header.Set("X-Trim-Userid", "1000")
		r.SetPathValue("id", tc.id)
		r.SetPathValue("action", "retry")
		w := httptest.NewRecorder()
		h.taskAction(w, r)
		if w.Code != tc.status {
			t.Fatal(tc.id, w.Code, w.Body.String())
		}
	}
	items, _ := db.List(ctx)
	var retried engine.Task
	for _, task := range items {
		if task.ID == "failed" {
			retried = task
		}
	}
	if len(items) != 2 || retried.ID != "failed" || retried.Status != engine.StatusQueued || retried.Error != "" {
		t.Fatal("retry created a replacement or lost queue state", items)
	}
	if len(e.starts) != 0 {
		t.Fatal("retry bypassed background concurrency scheduler")
	}
}
