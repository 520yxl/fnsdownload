package httpengine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

func TestRestoredTaskResumesWithRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=3-" {
			t.Errorf("expected range bytes=3-, got %q", got)
		}
		w.Header().Set("Content-Range", "bytes 3-5/6")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("def"))
	}))
	defer server.Close()
	dir := t.TempDir()
	part := filepath.Join(dir, "archive.bin.part")
	if err := os.WriteFile(part, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	e := New()
	task := engine.Task{ID: "restored-1", Engine: "http", Source: server.URL, Name: "archive.bin", Directory: dir, Status: engine.StatusRunning, CreatedAt: time.Now().UTC()}
	task = e.RestoreTask(task)
	if task.Status != engine.StatusPaused || task.CompletedBytes != 3 {
		t.Fatalf("task not reconciled: %#v", task)
	}
	if err := e.Resume(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := e.Get(context.Background(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == engine.StatusCompleted {
			break
		}
		if current.Status == engine.StatusFailed {
			t.Fatalf("download failed: %s", current.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, _ := e.Get(context.Background(), task.ID)
	if current.Status != engine.StatusCompleted {
		t.Fatalf("download did not complete: %#v", current)
	}
	data, err := os.ReadFile(filepath.Join(dir, "archive.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("unexpected file content %q", data)
	}
	if current.CompletedBytes != 6 {
		t.Fatalf("expected 6 completed bytes, got %d (%s)", current.CompletedBytes, fmt.Sprint(current.CompletedBytes))
	}
}
func TestPauseWaitsForHTTPTransferToStop(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	e := New()
	defer e.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task, err := e.Create(ctx, engine.CreateRequest{URL: server.URL + "/test.bin", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("download did not start")
	}
	if err = e.Pause(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.Get(ctx, task.ID)
	if err != nil || got.Status != engine.StatusPaused {
		t.Fatal("Pause returned before transfer stopped", got, err)
	}
}
