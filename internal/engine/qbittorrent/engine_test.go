package qbittorrent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceIdentity(t *testing.T) {
	h := strings.Repeat("ab", 20)
	for _, s := range []string{"magnet:?dn=one&xt=urn:btih:" + h, "magnet:?xt=urn:btih:" + strings.ToUpper(h)} {
		if got, e := MagnetHash(s); e != nil || got != h {
			t.Fatal(got, e)
		}
	}
	for _, s := range []string{"http://example.com/a", "magnet:?xt=urn:btih:nope", "magnet:?dn=only"} {
		if _, e := MagnetHash(s); e == nil {
			t.Fatal("bad magnet accepted")
		}
	}
	info := []byte("d6:lengthi3e4:name1:a12:piece lengthi16384e6:pieces20:12345678901234567890e")
	data := append(append([]byte("d4:info"), info...), 'e')
	sum := sha1.Sum(info)
	got, e := TorrentHash(data)
	if e != nil || got != hex.EncodeToString(sum[:]) {
		t.Fatal(got, e)
	}
	for _, b := range [][]byte{nil, []byte("d4:info0:e"), []byte("d4:infod1:a1:beeEXTRA"), []byte(strings.Repeat("l", 66) + strings.Repeat("e", 66))} {
		if _, e := TorrentHash(b); e == nil {
			t.Fatal("invalid torrent accepted")
		}
	}
}
func TestOptionsPersistence(t *testing.T) {
	e := NewManaged(Config{StateDir: t.TempDir()})
	o := DefaultOptions()
	o.UploadKB = 512
	if err := e.Configure(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if NewManaged(e.cfg).Options() != o {
		t.Fatal("not persistent")
	}
	o.Connections = 0
	if e.Configure(context.Background(), o) == nil {
		t.Fatal("invalid accepted")
	}
	if e.process != nil {
		t.Fatal("settings started process")
	}
}
func TestPrivateRequestsAndOwnership(t *testing.T) {
	ctx := context.Background()
	hash := strings.Repeat("ab", 20)
	var state = "stoppedDL"
	var dir string
	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/add") {
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Error(err)
			}
			dir = r.FormValue("savepath")
			if r.FormValue("stopped") != "true" {
				t.Error("task autostarted")
			}
			fmt.Fprint(w, "Ok.")
			return
		}
		_ = r.ParseForm()
		if strings.HasSuffix(r.URL.Path, "/info") {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"hash": hash, "name": "sample", "state": state, "save_path": dir, "size": 100, "completed": 25, "dlspeed": 3}})
			return
		}
		if r.FormValue("hashes") != hash {
			t.Error("action not hash-scoped")
		}
		actions = append(actions, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/start") {
			state = "downloading"
		}
		if strings.HasSuffix(r.URL.Path, "/stop") {
			state = "stoppedDL"
		}
	}))
	defer server.Close()
	e := NewManaged(Config{StateDir: t.TempDir()})
	e.endpoint = server.URL
	e.process = &managedProcess{done: make(chan struct{})}
	task, err := e.Create(ctx, engine.CreateRequest{URL: "magnet:?xt=urn:btih:" + hash, Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(task.Directory), "MoxYi-qB-") {
		t.Fatal("not isolated")
	}
	if err = e.Start(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	live, err := e.Get(ctx, task.ID)
	if err != nil || live.Status != engine.StatusRunning || live.CompletedBytes != 25 {
		t.Fatal(live, err)
	}
	if e.Delete(ctx, "qb-"+strings.Repeat("cd", 20), true) != engine.ErrNotFound {
		t.Fatal("foreign delete accepted")
	}
	if err = e.Pause(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Delete(ctx, task.ID, false); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 3 {
		t.Fatal(actions)
	}
	if _, err = os.Stat(dir); err != nil {
		t.Fatal("record deletion removed files")
	}
}
func TestPasswordHash(t *testing.T) {
	got := passwordHash("test-password", []byte("0123456789abcdef"))
	if got != "MDEyMzQ1Njc4OWFiY2RlZg==:n28SDebZuprx8qQC1ab0VzM017E7cLHYOT5c+rg567e1gWgb/3dfeyduocDNZyea3E80BniQU4Ze1fz2SDqNrw==" {
		t.Fatal(got)
	}
}
