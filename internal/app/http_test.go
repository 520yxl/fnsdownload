package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayPrefixServesHealthAndUI(t *testing.T) {
	root := t.TempDir()
	ui := filepath.Join(root, "ui")
	if err := os.MkdirAll(ui, 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(ui, "index.html"), []byte("<!doctype html><title>MoxYi</title>"), 0644); err != nil { t.Fatal(err) }
	h := NewHandler(Config{
		DataDir: filepath.Join(root, "data"),
		DownloadDir: filepath.Join(root, "downloads"),
		UIPath: ui,
		GatewayPrefix: "/app/fnsdownload",
	})

	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/app/fnsdownload/api/health", nil))
	if health.Code != http.StatusOK { t.Fatalf("health status = %d, body=%s", health.Code, health.Body.String()) }
	var payload map[string]any
	if err := json.Unmarshal(health.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
	if payload["ok"] != true || payload["service"] != "fnsdownload" { t.Fatalf("unexpected health response: %#v", payload) }

	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/app/fnsdownload/", nil))
	if page.Code != http.StatusOK { t.Fatalf("UI status = %d, body=%s", page.Code, page.Body.String()) }
	if page.Body.String() == "" { t.Fatal("UI response is empty") }
}
