//go:build linux

package aria2

import (
	"context"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndependentInstances(t *testing.T) {
	binary := os.Getenv("ARIA2_TEST_BINARY")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("aria2c")
		if err != nil {
			t.Skip("aria2 runtime not installed")
		}
	}
	binary, _ = filepath.Abs(binary)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Both instances run real aria2. Closing one must not touch the other's task.
	other := NewManaged(Config{Binary: binary, StateDir: filepath.Join(t.TempDir(), "other-app")})
	defer other.Close()
	own := NewManaged(Config{Binary: binary, StateDir: filepath.Join(t.TempDir(), "moxyi")})
	defer own.Close()
	if err := other.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if err := own.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if other.endpoint == own.endpoint || other.secret == own.secret {
		t.Fatal("instances share RPC identity")
	}
	twin := NewManaged(own.cfg)
	if err := twin.Health(ctx); err == nil {
		t.Fatal("second instance acquired same state directory")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/held.bin" {
			w.Header().Set("Content-Length", "1073741824")
			for {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(20 * time.Millisecond):
					if _, err := fmt.Fprint(w, strings.Repeat("y", 4096)); err != nil {
						return
					}
					w.(http.Flusher).Flush()
				}
			}
		}
		w.Header().Set("Content-Length", "16384")
		fmt.Fprint(w, strings.Repeat("x", 16384))
	}))
	defer func() { _ = other.Close(); server.Close() }()
	root := t.TempDir()
	sentinel := filepath.Join(root, "file.bin")
	if err := os.WriteFile(sentinel, []byte("other application file"), 0600); err != nil {
		t.Fatal(err)
	}
	foreign, err := other.Create(ctx, engine.CreateRequest{URL: server.URL + "/held.bin", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	task, err := own.Create(ctx, engine.CreateRequest{URL: server.URL + "/file.bin", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if task.Directory == foreign.Directory || task.Directory == root {
		t.Fatal("download control files share a directory")
	}
	if _, err = own.Get(ctx, foreign.ID); err == nil {
		t.Fatal("other instance task accepted")
	}
	if err = other.Start(ctx, foreign.ID); err != nil {
		t.Fatal(err)
	}
	wrongToken := own.secret
	own.secret = other.secret
	if err = own.call(ctx, "aria2.getVersion", nil, nil); err == nil {
		t.Fatal("cross-instance token accepted")
	}
	own.secret = wrongToken
	if err = own.Start(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		task, err = own.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == engine.StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real aria2 HTTP download did not finish", task.Status)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if data, err := os.ReadFile(filepath.Join(task.Directory, task.Name)); err != nil || len(data) != 16384 {
		t.Fatal("downloaded data mismatch", err)
	}
	if err = own.Delete(ctx, task.ID, true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(sentinel); string(data) != "other application file" {
		t.Fatal("unrelated file modified")
	}
	// Keep a paused task to verify private session restore and stable GID.
	paused, err := own.Create(ctx, engine.CreateRequest{URL: server.URL + "/paused.bin", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	cfg := own.cfg
	if err = own.Close(); err != nil {
		t.Fatal(err)
	}
	if err = other.Health(ctx); err != nil {
		t.Fatal("closing own instance stopped other application", err)
	}
	if got, err := other.Get(ctx, foreign.ID); err != nil || got.Status != engine.StatusRunning {
		t.Fatal("other task changed", err)
	}
	restarted := NewManaged(cfg)
	defer restarted.Close()
	restarted.RestoreTask(paused)
	if err = restarted.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := restarted.Get(ctx, paused.ID); err != nil || got.Status != engine.StatusPaused {
		t.Fatal("session not restored paused", err)
	}
	config, _ := os.ReadFile(filepath.Join(cfg.StateDir, "runtime.conf"))
	if !strings.Contains(string(config), "force-save=false") || !strings.Contains(string(config), "rpc-listen-all=false") {
		t.Fatal("unsafe session or RPC defaults")
	}
	if mode, err := os.Stat(filepath.Join(cfg.StateDir, "runtime.conf")); err != nil || mode.Mode().Perm()&0077 != 0 {
		t.Fatal("RPC configuration not private")
	}
}

func TestRetryFailedDownloadKeepsIdentity(t *testing.T) {
	binary := os.Getenv("ARIA2_TEST_BINARY")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("aria2c")
		if err != nil {
			t.Skip("aria2 runtime not installed")
		}
	}
	binary, _ = filepath.Abs(binary)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var available atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available.Load() {
			w.WriteHeader(403)
			return
		}
		fmt.Fprint(w, "retry succeeded")
	}))
	defer server.Close()
	e := NewManaged(Config{Binary: binary, StateDir: filepath.Join(t.TempDir(), "state")})
	defer e.Close()
	task, err := e.Create(ctx, engine.CreateRequest{URL: server.URL + "/retry.bin", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	wait := func(status engine.TaskStatus) engine.Task {
		t.Helper()
		for ctx.Err() == nil {
			got, err := e.Get(ctx, task.ID)
			if err == nil && got.Status == status {
				return got
			}
			if err == nil && status == engine.StatusCompleted && got.Status == engine.StatusFailed {
				var detail map[string]any
				_ = e.call(ctx, "aria2.tellStatus", []any{strings.TrimPrefix(task.ID, "aria2-"), []string{"status", "errorCode", "errorMessage", "files"}}, &detail)
				t.Fatalf("retry failed: %v", detail)
			}

			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("task did not reach", status)
		return engine.Task{}
	}
	wait(engine.StatusFailed)
	available.Store(true)
	if err = e.Start(ctx, task.ID); err != nil {
		t.Fatal("retry", err)
	}
	done := wait(engine.StatusCompleted)
	if done.ID != task.ID || done.Directory != task.Directory {
		t.Fatal("retry changed task identity or directory")
	}
	data, err := os.ReadFile(filepath.Join(done.Directory, done.Name))
	if err != nil || string(data) != "retry succeeded" {
		t.Fatal("retry file incorrect", err)
	}
}

func TestSettingsAndMissingRecordRecoveryWithRealAria2(t *testing.T) {
	binary, err := exec.LookPath("aria2c")
	if configured := os.Getenv("ARIA2_TEST_BINARY"); configured != "" {
		binary = configured
		err = nil
	}
	if err != nil {
		t.Skip("aria2 runtime not installed")
	}
	binary, _ = filepath.Abs(binary)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	own := NewManaged(Config{Binary: binary, StateDir: filepath.Join(t.TempDir(), "own")})
	defer own.Close()
	other := NewManaged(Config{Binary: binary, StateDir: filepath.Join(t.TempDir(), "other")})
	defer other.Close()
	if err = own.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if err = other.Health(ctx); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Split = 2
	options.Connections = 2
	options.DownloadKB = 4096
	if err = own.Configure(ctx, options); err != nil {
		t.Fatal(err)
	}
	var global map[string]string
	if err = other.call(ctx, "aria2.getGlobalOption", nil, &global); err != nil {
		t.Fatal(err)
	}
	if global["max-connection-per-server"] != "8" || global["max-overall-download-limit"] != "0" {
		t.Fatal("settings affected other application", global["max-connection-per-server"], global["max-overall-download-limit"])
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("verified", 8192)) }))
	defer server.Close()
	task, err := own.Create(ctx, engine.CreateRequest{URL: server.URL + "/test.bin", Directory: t.TempDir(), Name: "test.bin"})
	if err != nil {
		t.Fatal(err)
	}
	gid := strings.TrimPrefix(task.ID, "aria2-")
	var actual map[string]string
	if err = own.call(ctx, "aria2.getOption", []any{gid}, &actual); err != nil {
		t.Fatal(err)
	}
	if actual["split"] != "2" || actual["max-connection-per-server"] != "2" {
		t.Fatal("task did not inherit configured connections", actual["split"], actual["max-connection-per-server"])
	}
	if err = own.Pause(ctx, task.ID); err != nil {
		t.Fatal("idempotent pause failed", err)
	}
	// Simulate a session/database mismatch for this owned, paused task only.
	if err = own.call(ctx, "aria2.forceRemove", []any{gid}, nil); err != nil {
		t.Fatal(err)
	}
	for {
		var status struct {
			Status string `json:"status"`
		}
		if err = own.call(ctx, "aria2.tellStatus", []any{gid}, &status); err != nil {
			if missingTask(err) {
				break
			}
			t.Fatal(err)
		}
		if status.Status == "removed" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err = own.call(ctx, "aria2.removeDownloadResult", []any{gid}, nil); err != nil && !missingTask(err) {
		t.Fatal(err)
	}
	missing, err := own.Get(ctx, task.ID)
	if err != nil || missing.Status != engine.StatusPaused || !strings.Contains(missing.Error, "任务记录未加载") {
		t.Fatal("real RPC 400 not recovered as actionable paused task", missing, err)
	}
	if err = own.Start(ctx, task.ID); err != nil {
		t.Fatal("missing task resume failed", err)
	}
	for {
		task, err = own.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == engine.StatusCompleted {
			break
		}
		if task.Status == engine.StatusFailed {
			t.Fatal(task.Error)
		}
		select {
		case <-ctx.Done():
			t.Fatal("recovered transfer did not finish", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	data, err := os.ReadFile(filepath.Join(task.Directory, task.Name))
	if err != nil || string(data) != strings.Repeat("verified", 8192) {
		t.Fatal("recovered download data mismatch", err)
	}
	reloaded := NewManaged(own.cfg)
	if reloaded.Options() != options {
		t.Fatal("settings not persisted")
	}
}
