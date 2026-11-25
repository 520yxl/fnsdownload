package app

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestXunleiRoutesRequireGatewayUser(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir, DatabasePath: filepath.Join(dir, "tasks.db"), DownloadDir: filepath.Join(dir, "downloads")}
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, item := range []struct{ method, path, body string }{
		{"GET", "/api/xunlei/settings", ""}, {"PUT", "/api/xunlei/settings", `{"concurrent":3}`},
		{"GET", "/api/xunlei/tasks/xunlei-1/details", ""}, {"POST", "/api/xunlei/recover", ""},
		{"POST", "/api/xunlei/batch", `{"urls":["https://example.com/a"]}`},
	} {
		req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("%s accepted unauthenticated request: %d", item.path, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/api/xunlei/settings", nil)
	req.Header.Set("X-Trim-Userid", "1000")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"concurrent":10`) {
		t.Fatalf("default settings: %d %s", w.Code, w.Body.String())
	}
}
