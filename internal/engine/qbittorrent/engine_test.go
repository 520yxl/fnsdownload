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
			fmt.Fprintf(w, `{"success_count":1,"failure_count":0,"pending_count":0,"added_torrent_ids":["%s"]}`, hash)
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

func TestV2IdentityRetainsFullMagnet(t *testing.T) {
	data := []byte("d4:infod12:meta versioni2e4:name1:aee")
	hash, err := TorrentHash(data)
	if err != nil {
		t.Fatal(err)
	}
	source, err := TorrentMagnet(data)
	if err != nil || !strings.Contains(source, "urn:btmh:1220") || len(strings.TrimPrefix(source, "magnet:?xt=urn:btmh:1220")) != 64 {
		t.Fatal(source, err)
	}
	got, err := MagnetHash(source)
	if err != nil || got != hash {
		t.Fatal(got, err)
	}
}
func TestExpiredCookieRenewsUsingActualName(t *testing.T) {
	ctx := context.Background()
	login := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/login") {
			login++
			_ = r.ParseForm()
			if r.FormValue("password") != "private" {
				t.Error("wrong login")
			}
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_random", Value: "session", Path: "/"})
			fmt.Fprint(w, "Ok.")
			return
		}
		if cookie, err := r.Cookie("QBT_SID_random"); err != nil || cookie.Value != "session" {
			w.WriteHeader(403)
			return
		}
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()
	e := NewManaged(Config{StateDir: t.TempDir()})
	e.endpoint = server.URL
	e.password = "private"
	var rows []torrent
	if err := e.form(ctx, "torrents/info", nil, &rows); err != nil {
		t.Fatal(err)
	}
	if login != 1 {
		t.Fatal("login retry not bounded", login)
	}
}

func TestRestoredWildcardNeverReachesAPI(t *testing.T) {
	e := NewManaged(Config{StateDir: t.TempDir()})
	for _, id := range []string{"all", "qb-all", "qb-abc|def", "qb-" + strings.Repeat("a", 40) + "|all"} {
		e.RestoreTask(engine.Task{ID: id, Engine: "qbittorrent"})
		if e.Start(context.Background(), id) != engine.ErrNotFound || e.Delete(context.Background(), id, true) != engine.ErrNotFound {
			t.Fatal("invalid hash reached API", id)
		}
	}
}

func TestParkResumeFlagsWithoutChangingPayload(t *testing.T) {
	raw := []byte("d12:auto_managedi1e4:info3:abc6:pausedi0ee")
	got, err := pauseResumeBytes(raw)
	if err != nil || string(got) != "d12:auto_managedi0e4:info3:abc6:pausedi1ee" {
		t.Fatal(string(got), err)
	}
	root := t.TempDir()
	backup := filepath.Join(root, "qBittorrent", "data", "BT_backup")
	_ = os.MkdirAll(backup, 0700)
	path := filepath.Join(backup, strings.Repeat("ab", 20)+".fastresume")
	_ = os.WriteFile(path, raw, 0600)
	e := NewManaged(Config{StateDir: root})
	if err = e.parkResumeData(); err != nil {
		t.Fatal(err)
	}
	parked, _ := os.ReadFile(path)
	if string(parked) != string(got) {
		t.Fatal("resume flags not persisted")
	}
	if _, err = pauseResumeBytes([]byte("d6:pausedi0e6:pausedi1ee")); err == nil {
		t.Fatal("ambiguous resume accepted")
	}
}
