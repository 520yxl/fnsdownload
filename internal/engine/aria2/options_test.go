package aria2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/520yxl/fnsdownload/internal/engine"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptionsValidationAndPersistence(t *testing.T) {
	root := t.TempDir()
	e := NewManaged(Config{StateDir: root})
	wanted := DefaultOptions()
	wanted.Connections = 4
	wanted.Split = 12
	wanted.DownloadKB = 2048
	if err := e.Configure(context.Background(), wanted); err != nil {
		t.Fatal(err)
	}
	if e.process != nil {
		t.Fatal("saving settings launched a process")
	}
	reopened := NewManaged(Config{StateDir: root})
	if reopened.Options() != wanted || reopened.settingsError != nil {
		t.Fatal("settings did not persist", reopened.Options(), reopened.settingsError)
	}
	bad := wanted
	bad.Connections = 17
	if err := e.Configure(context.Background(), bad); err == nil {
		t.Fatal("invalid connections accepted")
	}
	if e.Options() != wanted {
		t.Fatal("invalid setting changed configuration")
	}
	data, _ := os.ReadFile(filepath.Join(root, "settings.json"))
	if strings.Contains(string(data), "rpc-secret") {
		t.Fatal("RPC credential persisted in product settings")
	}
	for _, change := range []func(*Options){func(o *Options) { o.MinSplitMB = 0 }, func(o *Options) { o.Split = 0 }, func(o *Options) { o.Timeout = 0 }, func(o *Options) { o.DownloadKB = -1 }, func(o *Options) { o.MaxTries = 0 }, func(o *Options) { o.RetryWait = -1 }} {
		o := DefaultOptions()
		change(&o)
		if o.Validate() == nil {
			t.Fatal("invalid options accepted", o)
		}
	}
}

func TestRPC400ErrorsAreDecodedAndRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{"id": request.ID, "jsonrpc": "2.0", "error": map[string]any{"code": 1, "message": "GID abc is not found; token:private-credential https://example.com/?secret=private"}})
	}))
	defer server.Close()
	e := NewManaged(Config{})
	e.endpoint = server.URL
	e.secret = "private-credential"
	err := e.call(context.Background(), "aria2.tellStatus", []any{"abc"}, nil)
	if !missingTask(err) {
		t.Fatal("HTTP 400 discarded JSON-RPC error", err)
	}
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "example.com") {
		t.Fatal("sensitive server error reflected", err)
	}
	if !strings.Contains(err.Error(), "任务记录未加载") {
		t.Fatal("error not actionable", err)
	}
}

func TestMissingOwnedTaskRebuildsOnResume(t *testing.T) {
	ctx := context.Background()
	exists := false
	adds := 0
	unpauses := 0
	gid := "0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID, Method string
			Params     []json.RawMessage
		}
		json.NewDecoder(r.Body).Decode(&request)
		var result any = "OK"
		switch request.Method {
		case "aria2.getVersion":
			result = map[string]string{"version": "test"}
		case "aria2.tellStatus":
			if !exists {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]any{"id": request.ID, "error": map[string]any{"code": 1, "message": "GID " + gid + " is not found"}})
				return
			}
			result = map[string]any{"status": "paused", "completedLength": "128", "totalLength": "1024"}
		case "aria2.addUri":
			adds++
			var options map[string]string
			json.Unmarshal(request.Params[2], &options)
			if options["gid"] != gid || options["continue"] != "true" || options["allow-overwrite"] != "false" || options["split"] != "8" {
				t.Error("unsafe rebuild options", options)
			}
			exists = true
			result = gid
		case "aria2.unpause":
			unpauses++
		case "aria2.changeOption":
		default:
			t.Error("unexpected RPC", request.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": request.ID, "jsonrpc": "2.0", "result": result})
	}))
	defer server.Close()
	e := NewManaged(Config{StateDir: t.TempDir()})
	e.endpoint = server.URL
	e.secret = "test"
	e.process = &managedProcess{done: make(chan struct{})}
	dir, err := os.MkdirTemp(t.TempDir(), "MoxYi-aria2-")
	if err != nil {
		t.Fatal(err)
	}
	task := engine.Task{ID: "aria2-" + gid, Engine: "aria2", Source: "https://example.com/file.bin", Name: "file.bin", Directory: dir, Status: engine.StatusPaused, CompletedBytes: 128}
	e.tasks[task.ID] = task
	cached, err := e.Get(ctx, task.ID)
	if err != nil || cached.Status != engine.StatusPaused || cached.CompletedBytes != 128 || cached.Error == "" {
		t.Fatal("missing record lost cached progress", cached, err)
	}
	if err = e.Start(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if adds != 1 || unpauses != 1 {
		t.Fatal("resume did not rebuild before starting", adds, unpauses)
	}
	if err = e.Start(ctx, "foreign-task"); !errors.Is(err, engine.ErrNotFound) {
		t.Fatal("foreign task accepted", err)
	}
}
