package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNASTorrentRequiresGrantReadACLAndBoundedRegularFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.torrent")
	data := []byte("d4:infod4:name1:aee")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRIM_API_TOKEN", "test-token")
	old := fnosHTTP
	t.Cleanup(func() { fnosHTTP = old })
	granted, readable := true, true
	fnosHTTP = &http.Client{Transport: fnosRoundTrip(func(r *http.Request) (*http.Response, error) {
		var req struct {
			Req  string `json:"req"`
			Data struct {
				UID  int    `json:"uid"`
				Path string `json:"path"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Data.UID != 1000 {
			t.Fatal("wrong user")
		}
		var result any
		switch req.Req {
		case "trim.file.getUserAccessibleFolders":
			paths := []string{}
			if granted {
				paths = append(paths, root)
			}
			result = map[string]any{"paths": paths}
		case "trim.file.checkUserACL":
			result = []map[string]any{{"path": req.Data.Path, "readable": readable, "writable": false}}
		default:
			t.Fatal("unexpected method")
		}
		body, _ := json.Marshal(map[string]any{"code": 0, "data": result})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	read := func() error { _, err := readNASTorrent(context.Background(), "1000", path); return err }
	got, err := readNASTorrent(context.Background(), "1000", path)
	if err != nil || string(got) != string(data) {
		t.Fatal("read-only authorized file rejected", err)
	}
	granted = false
	if read() == nil {
		t.Fatal("ungranted file read")
	}
	granted = true
	readable = false
	if read() == nil {
		t.Fatal("ACL denial ignored")
	}
	readable = true
	if _, err := readNASTorrent(context.Background(), "invalid", path); err == nil {
		t.Fatal("invalid user accepted")
	}
	if _, err := readNASTorrent(context.Background(), "1000", root); err == nil {
		t.Fatal("directory accepted")
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Truncate((8 << 20) + 1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if read() == nil {
		t.Fatal("oversized torrent accepted")
	}
}

