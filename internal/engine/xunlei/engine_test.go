package xunleiengine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
	"github.com/520yxl/fnsdownload/internal/xunlei"
)

type fakeSDK struct {
	state           xunlei.TaskState
	removed, closed bool
	removeFiles     bool
	fail            error
}

func (f *fakeSDK) Init(string, string, string) error { return nil }
func (f *fakeSDK) Login(string) error                { return nil }
func (f *fakeSDK) CreateP2SP(string, string, string) (uint64, error) {
	return 18446744073709551610, f.fail
}
func (f *fakeSDK) Start(uint64) error { f.state.StateCode = 5; return f.fail }
func (f *fakeSDK) Stop(uint64) error  { f.state.StateCode = 7; return f.fail }
func (f *fakeSDK) Delete(_ uint64, remove bool) error {
	f.removed = true
	f.removeFiles = remove
	return f.fail
}
func (f *fakeSDK) State(uint64) (xunlei.TaskState, error) { return f.state, f.fail }
func (f *fakeSDK) Close() error                           { f.closed = true; return nil }

func TestTaskLifecycleAndSDKFailures(t *testing.T) {
	fake := &fakeSDK{}
	e := New(Config{AppID: "test", AuthURL: "https://auth.invalid", StateDir: t.TempDir()})
	e.sdk = fake
	e.compiled = true
	e.initialized = true
	e.expires = time.Now().Add(time.Hour)
	ctx := context.Background()
	task, err := e.Create(ctx, engine.CreateRequest{URL: "https://example.com/file.zip", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "xunlei-18446744073709551610" {
		t.Fatal("SDK 64-bit ID lost precision")
	}
	if err := e.Start(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	fake.state = xunlei.TaskState{StateCode: 5, Speed: 30, TotalSize: 100, DownloadedSize: 40}
	got, err := e.Get(ctx, task.ID)
	if err != nil || got.Status != engine.StatusRunning || got.CompletedBytes != 40 || got.SpeedBytes != 30 {
		t.Fatalf("bad SDK state: %+v %v", got, err)
	}
	if err := e.Pause(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = e.Get(ctx, task.ID)
	if got.Status != engine.StatusPaused || got.SpeedBytes != 0 {
		t.Fatal("pause not reflected")
	}
	if err := e.Resume(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	e.lastError = errors.New("authorization unavailable")
	e.retryAfter = time.Now().Add(time.Hour)
	if err := e.Pause(ctx, task.ID); err != nil {
		t.Fatal("authorization outage blocked pause")
	}
	if _, err := e.Get(ctx, task.ID); err != nil {
		t.Fatal("authorization outage blocked task state")
	}
	if err := e.Resume(ctx, task.ID); err == nil {
		t.Fatal("resume accepted invalid authorization")
	}
	e.lastError = nil

	fake.fail = errors.New("SDK operation failed")
	if err := e.Delete(ctx, task.ID, false); err == nil {
		t.Fatal("SDK deletion failure swallowed")
	}
	if _, ok := e.tasks[task.ID]; !ok {
		t.Fatal("failed deletion lost task")
	}
	fake.fail = nil
	if err := e.Delete(ctx, task.ID, false); err != nil {
		t.Fatal(err)
	}
	if fake.removeFiles {
		t.Fatal("record-only removal deleted user files")
	}
	if _, err := e.Get(ctx, task.ID); !errors.Is(err, engine.ErrNotFound) {
		t.Fatal("deleted task still present")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if !fake.closed {
		t.Fatal("SDK not released")
	}
}

func TestTaskValidationAndRestore(t *testing.T) {
	e := New(Config{})
	for _, req := range []engine.CreateRequest{
		{URL: "magnet:?xt=test", Directory: t.TempDir()},
		{URL: "https://example.com/a", Directory: t.TempDir(), Name: "../escape"},
		{URL: "https://example.com/a", Directory: "relative"},
	} {
		if _, err := e.Create(context.Background(), req); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	task := e.RestoreTask(engine.Task{ID: "xunlei-1", Engine: "xunlei", Status: engine.StatusRunning, SpeedBytes: 100})
	if task.Status != engine.StatusPaused || task.SpeedBytes != 0 {
		t.Fatal("restored task silently continued")
	}
	path := filepath.Join(t.TempDir(), "device-id")
	a, err := installationID(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := installationID(path)
	if err != nil || a != b || len(a) != 32 {
		t.Fatal("installation identity changed")
	}
}
