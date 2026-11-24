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
	webseed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	if err = b.Health(ctx); err != nil {
		t.Fatal("closing a affected b", err)
	}
	reopened := NewManaged(a.cfg)
	defer reopened.Close()
	reopened.RestoreTask(live)
	if err = reopened.Health(ctx); err != nil {
		t.Fatal("restart:", err)
	}
	restored, err := reopened.Get(ctx, task.ID)
	if err != nil || restored.CompletedBytes != int64(len(payload)) {
		t.Fatal("resume record lost", restored, err)
	}
	if err = reopened.Delete(ctx, task.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(task.Directory, "payload.bin")); err != nil {
		t.Fatal("delete record removed file")
	}
	if strings.Contains(task.Source, a.password) {
		t.Fatal("credential leaked into task")
	}
}
