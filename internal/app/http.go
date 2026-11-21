package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Handler struct {
	cfg Config
	mux *http.ServeMux
}

func NewHandler(cfg Config) http.Handler {
	_ = os.MkdirAll(cfg.DataDir, 0700)
	_ = os.MkdirAll(cfg.DownloadDir, 0750)
	h := &Handler{cfg: cfg, mux: http.NewServeMux()}
	h.routes()
	prefix := strings.TrimRight(cfg.GatewayPrefix, "/")
	if prefix == "" { return h.mux }
	return http.StripPrefix(prefix, h.mux)
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "fnsdownload", "version": "0.2.3", "runtime": "go"})
	})
	h.mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"download_dir": filepath.Clean(h.cfg.DownloadDir),
			"data_dir": filepath.Clean(h.cfg.DataDir),
		})
	})
	if info, err := os.Stat(filepath.Join(h.cfg.UIPath, "index.html")); err == nil && !info.IsDir() {
		h.mux.Handle("/", http.FileServer(http.Dir(h.cfg.UIPath)))
	} else {
		h.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "MoxYi Download UI assets are missing. Reinstall the FPK package.", http.StatusServiceUnavailable)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
