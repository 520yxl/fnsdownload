package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type fnosRoundTrip func(*http.Request) (*http.Response, error)

func (f fnosRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAuthorizedDownloadDirectory(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	old := fnosHTTP
	t.Cleanup(func() { fnosHTTP = old })
	t.Setenv("TRIM_API_TOKEN", "test-app-token")
	writeAllowed := true
	fnosHTTP = &http.Client{Transport: fnosRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer test-app-token" {
			t.Fatal("missing application credential")
		}
		var request struct {
			Req  string `json:"req"`
			Data struct {
				UID int `json:"uid"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Data.UID != 1000 {
			t.Fatal("must query authenticated gateway user")
		}
		var data any
		switch request.Req {
		case "trim.file.getUserAccessibleFolders":
			data = map[string]any{"paths": []string{root}}
		case "trim.file.checkUserACL":
			data = []map[string]any{{"path": root, "writable": writeAllowed}}
		default:
			t.Fatal("unexpected platform method")
		}
		body, _ := json.Marshal(map[string]any{"code": 0, "data": data})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	if got, err := authorizedDownloadDir(context.Background(), "1000", root); err != nil || got != root {
		t.Fatalf("authorized folder rejected: %v", err)
	}
	if _, err := authorizedDownloadDir(context.Background(), "1000", other); err == nil {
		t.Fatal("ungranted folder accepted")
	}
	writeAllowed = false
	if _, err := authorizedDownloadDir(context.Background(), "1000", root); err == nil {
		t.Fatal("read-only folder accepted")
	}
	if _, err := authorizedDownloadDir(context.Background(), "invalid", root); err == nil {
		t.Fatal("invalid user accepted")
	}
	t.Setenv("TRIM_API_TOKEN", "")
	if _, err := authorizedDownloadDir(context.Background(), "1000", root); err == nil {
		t.Fatal("missing platform token accepted")
	}
}

func TestDirectoryContainment(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{root, filepath.Join(root, "child")} {
		if !withinDirectory(root, path) {
			t.Fatal("child rejected")
		}
	}
	for _, path := range []string{root + "-other", filepath.Dir(root)} {
		if withinDirectory(root, path) {
			t.Fatal("outside folder accepted")
		}
	}
}
