package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayPrefixServesFrontendAndAPI(t *testing.T) {
	root := t.TempDir()
	ui := filepath.Join(root, "www")
	if err := os.MkdirAll(filepath.Join(ui, "assets"), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(ui, "index.html"), []byte("<html>moxyi gateway ui</html>"), 0644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(ui, "assets", "app.js"), []byte("window.moxyiGateway=true"), 0644); err != nil { t.Fatal(err) }

	cfg := Config{
		SocketPath: filepath.Join(root, "app.sock"),
		DataDir: filepath.Join(root, "data"),
		DatabasePath: filepath.Join(root, "data", "tasks.db"),
		DownloadDir: filepath.Join(root, "downloads"),
		UIPath: ui,
		GatewayPrefix: "/app/fnsdownload",
	}
	h, err := NewHandler(cfg)
	if err != nil { t.Fatal(err) }
	defer h.Close()

	cases := []struct{ path, want string; status int }{
		{"/app/fnsdownload/", "moxyi gateway ui", http.StatusOK},
		{"/app/fnsdownload/assets/app.js", "moxyiGateway", http.StatusOK},
		{"/app/fnsdownload/api/health", "\"service\":\"fnsdownload\"", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.status { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
			if !strings.Contains(rec.Body.String(), tc.want) { t.Fatalf("body does not contain %q: %s", tc.want, rec.Body.String()) }
		})
	}
}

func TestGatewayPrefixRejectsRequestsOutsidePrefix(t *testing.T) {
	root := t.TempDir()
	cfg := Config{
		SocketPath: filepath.Join(root, "app.sock"),
		DataDir: filepath.Join(root, "data"),
		DatabasePath: filepath.Join(root, "data", "tasks.db"),
		DownloadDir: filepath.Join(root, "downloads"),
		UIPath: filepath.Join(root, "www"),
		GatewayPrefix: "/app/fnsdownload",
	}
	h, err := NewHandler(cfg)
	if err != nil { t.Fatal(err) }
	defer h.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound { t.Fatalf("outside-prefix request status=%d, want 404", rec.Code) }
}
