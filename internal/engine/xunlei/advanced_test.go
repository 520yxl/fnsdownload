package xunleiengine

import (
	"context"
	"errors"
	"github.com/520yxl/fnsdownload/internal/xunlei"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type advancedFake struct {
	fakeSDK
	configured []xunlei.Options
	configFail bool
	batch      []xunlei.BatchFile
	headers    map[string]string
}

func (f *advancedFake) Configure(o xunlei.Options) error {
	f.configured = append(f.configured, o)
	if f.configFail {
		f.configFail = false
		return errors.New("vendor rejected setting")
	}
	return nil
}
func (f *advancedFake) TaskIDs(finished bool) ([]uint64, error) {
	if finished {
		return []uint64{44}, nil
	}
	return []uint64{43}, nil
}
func (f *advancedFake) Info(id uint64) (xunlei.TaskInfo, error) {
	return xunlei.TaskInfo{Directory: "/allowed", Name: "restored.bin", URL: "https://example.com/restored.bin"}, nil
}
func (f *advancedFake) CreateBatch(_ string, _ uint32, files []xunlei.BatchFile) (uint64, error) {
	f.batch = files
	return 42, nil
}
func (f *advancedFake) SetHeader(_ uint64, k, v string) error {
	if f.headers == nil {
		f.headers = map[string]string{}
	}
	f.headers[k] = v
	return nil
}
func (f *advancedFake) SetTaskToken(uint64, string) error { return nil }
func advancedEngine(t *testing.T) (*Engine, *advancedFake) {
	t.Helper()
	e := New(Config{AppID: "test", AuthURL: "https://auth.invalid", StateDir: t.TempDir()})
	f := &advancedFake{}
	e.sdk = f
	e.compiled = true
	e.initialized = true
	e.expires = time.Now().Add(time.Hour)
	return e, f
}
func TestSettingsPersistenceAndRollback(t *testing.T) {
	e, f := advancedEngine(t)
	ctx := context.Background()
	o := defaultOptions()
	o.Concurrent = 3
	o.DownloadKB = 64
	if err := e.Configure(ctx, o); err != nil {
		t.Fatal(err)
	}
	reloaded := New(e.cfg)
	if reloaded.settingsError != nil || reloaded.Options() != o {
		t.Fatal("settings not restored")
	}
	rejected := o
	rejected.Concurrent = 4
	f.configFail = true
	if err := e.Configure(ctx, rejected); err == nil {
		t.Fatal("vendor failure ignored")
	}
	if e.Options() != o || f.configured[len(f.configured)-1] != o {
		t.Fatal("settings not rolled back")
	}
	invalid := o
	invalid.DownloadKB = 1
	if err := e.Configure(ctx, invalid); err == nil {
		t.Fatal("invalid speed accepted")
	}
	// A failed disk write must restore runtime options too.
	if err := os.Remove(filepath.Join(e.cfg.StateDir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(e.cfg.StateDir, "settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.Configure(ctx, rejected); err == nil {
		t.Fatal("disk failure ignored")
	}
	if e.Options() != o || f.configured[len(f.configured)-1] != o {
		t.Fatal("disk failure changed runtime settings")
	}
}
func TestBatchHeadersAndRecovery(t *testing.T) {
	e, f := advancedEngine(t)
	ctx := context.Background()
	dir := t.TempDir()
	task, err := e.CreateBatch(ctx, "test", dir, []string{"https://example.com/a.zip", "https://example.com/b.zip"}, 2)
	if err != nil || len(f.batch) != 2 {
		t.Fatalf("batch failed: %v", err)
	}
	if err = e.SetHeaders(ctx, task.ID, map[string]string{"Referer": "https://example.com/"}); err != nil {
		t.Fatal(err)
	}
	if f.headers["Referer"] == "" {
		t.Fatal("header missing")
	}
	if err = e.SetHeaders(ctx, task.ID, map[string]string{"Referer": "value\r\nInjected: yes"}); err == nil {
		t.Fatal("header injection accepted")
	}
	if _, err = e.CreateBatch(ctx, "test", dir, []string{"https://a.com/file.zip", "https://b.com/file.zip"}, 2); err == nil {
		t.Fatal("duplicate destination accepted")
	}
	denied, err := e.Recover(ctx, func(string) error { return errors.New("not authorized") })
	if err != nil || len(denied) != 0 {
		t.Fatal("recovery ignored authorization")
	}
	recovered, err := e.Recover(ctx, func(path string) error {
		if path != "/allowed" {
			return errors.New("bad path")
		}
		return nil
	})
	if err != nil || len(recovered) != 2 {
		t.Fatalf("recovery failed: %v", err)
	}
	if _, err = e.Recover(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(e.tasks) != 3 {
		t.Fatal("recovery duplicated tasks")
	}
}
