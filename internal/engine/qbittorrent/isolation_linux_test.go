//go:build linux && amd64

package qbittorrent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedIsolationAndTorrentDownload(t *testing.T) {
	binary := os.Getenv("QBITTORRENT_TEST_BINARY")
	if binary == "" {
		t.Skip("requires packaged native qBittorrent")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	a := NewManaged(Config{Binary: binary, StateDir: filepath.Join(root, "a")})
	b := NewManaged(Config{Binary: binary, StateDir: filepath.Join(root, "b")})
	defer a.Close()
	defer b.Close()
	o := DefaultOptions()
	o.DHT = false
	o.PEX = false
	if err := a.Configure(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := b.Configure(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := a.Health(ctx); err != nil {
		t.Fatal("a:", err)
	}
	if err := b.Health(ctx); err != nil {
		t.Fatal("b:", err)
	}
	if a.endpoint == b.endpoint || a.password == b.password {
		t.Fatal("instances share identity")
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", a.endpoint+"/api/v2/torrents/info", nil)
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 && resp.StatusCode != 401 {
		t.Fatal("unauthenticated loopback request accepted", resp.StatusCode)
	}
	payload := bytes.Repeat([]byte("qBittorrent native download verification\n"), 300)
	sum := sha1.Sum(payload)
	info := []byte(fmt.Sprintf("d6:lengthi%de4:name11:payload.bin12:piece lengthi16384e6:pieces20:", len(payload)))
	info = append(info, sum[:]...)
	info = append(info, 'e')
	var sourceEnabled atomic.Bool
	sourceEnabled.Store(true)
	webseed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sourceEnabled.Load() {
			w.WriteHeader(503)
			return
		}
		http.ServeContent(w, r, "payload.bin", time.Now(), bytes.NewReader(payload))
	}))
	defer webseed.Close()
	url := webseed.URL + "/payload.bin"
	data := append([]byte("d4:info"), info...)
	data = append(data, []byte(fmt.Sprintf("8:url-list%d:%se", len(url), url))...)
	task, err := a.Create(ctx, engine.CreateRequest{Torrent: data, Directory: root})
	if err != nil {
		t.Fatal("create:", err)
	}
	if err = b.Delete(ctx, task.ID, true); err != engine.ErrNotFound {
		t.Fatal("foreign delete accepted", err)
	}
	if err = a.Start(ctx, task.ID); err != nil {
		t.Fatal("start:", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var live engine.Task
	for time.Now().Before(deadline) {
		live, err = a.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if live.Status == engine.StatusCompleted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if live.Status != engine.StatusCompleted {
		t.Fatalf("download incomplete: %+v", live)
	}
	got, err := os.ReadFile(filepath.Join(task.Directory, "payload.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("download integrity", err)
	}
	sourceEnabled.Store(false)
	stalled, err := a.Create(ctx, engine.CreateRequest{URL: "magnet:?xt=urn:btih:" + strings.Repeat("cd", 20), Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Start(ctx, stalled.ID); err != nil {
		t.Fatal("metadata start:", err)
	}
	if err = a.Pause(ctx, stalled.ID); err != nil {
		t.Fatal("metadata pause:", err)
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	if err = b.Health(ctx); err != nil {
		t.Fatal("closing a affected b", err)
	}
	reopened := NewManaged(a.cfg)
	defer reopened.Close()
	reopened.RestoreTask(live)
	reopened.RestoreTask(stalled)
	if err = reopened.Health(ctx); err != nil {
		t.Fatal("restart:", err)
	}
	// libtorrent verifies restored files asynchronously; the source is disabled
	// so passing cannot be explained by silently downloading the bytes again.
	var restored, paused engine.Task
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		restored, err = reopened.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		paused, err = reopened.Get(ctx, stalled.ID)
		if err != nil {
			t.Fatal(err)
		}
		if restored.Status == engine.StatusCompleted && restored.CompletedBytes == int64(len(payload)) && paused.Status == engine.StatusPaused {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if restored.Status != engine.StatusCompleted || restored.CompletedBytes != int64(len(payload)) {
		t.Fatal("resume record lost after verification with source disabled", restored)
	}
	if paused.Status != engine.StatusPaused {
		t.Fatal("unfinished torrent did not restore paused", paused)
	}

	if err = reopened.Delete(ctx, task.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(task.Directory, "payload.bin")); err != nil {
		t.Fatal("delete record removed file")
	}
	sourceEnabled.Store(true)
	again, err := reopened.Create(ctx, engine.CreateRequest{Torrent: data, Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.Start(ctx, again.ID); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		r, e := reopened.Get(ctx, again.ID)
		if e != nil {
			t.Fatal(e)
		}
		if r.Status == engine.StatusCompleted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err = os.Stat(filepath.Join(again.Directory, "payload.bin")); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Delete(ctx, again.ID, true); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(filepath.Join(again.Directory, "payload.bin")); os.IsNotExist(err) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !os.IsNotExist(err) {
		t.Fatal("delete-files did not remove owned content")
	}
	if _, err = os.Stat(filepath.Join(task.Directory, "payload.bin")); err != nil {
		t.Fatal("delete-files affected retained earlier download")
	}
	if strings.Contains(task.Source, a.password) {
		t.Fatal("credential leaked into task")
	}
}
