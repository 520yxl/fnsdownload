package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
	aria2engine "github.com/520yxl/fnsdownload/internal/engine/aria2"
	httpengine "github.com/520yxl/fnsdownload/internal/engine/http"
	xunleiengine "github.com/520yxl/fnsdownload/internal/engine/xunlei"
	"github.com/520yxl/fnsdownload/internal/store"
)

type Handler struct {
	settingsMu   sync.Mutex
	cfg          Config
	mux          *http.ServeMux
	handler      http.Handler
	engines      *engine.Manager
	store        *store.Store
	httpEngine   *httpengine.Engine
	xunleiEngine *xunleiengine.Engine
	aria2Engine  *aria2engine.Engine
}

func NewHandler(cfg Config) (*Handler, error) {
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DownloadDir, 0750); err != nil {
		return nil, err
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	manager := engine.NewManager()
	httpAdapter := httpengine.New()
	if err := manager.Register(httpAdapter); err != nil {
		_ = db.Close()
		return nil, err
	}
	var aria2Adapter *aria2engine.Engine
	if cfg.Aria2Binary != "" {
		aria2Adapter = aria2engine.NewManaged(aria2engine.Config{Binary: cfg.Aria2Binary, StateDir: cfg.Aria2StateDir})
		if err := manager.Register(aria2Adapter); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	xunleiAdapter := xunleiengine.New(cfg.Xunlei)
	if err := manager.Register(xunleiAdapter); err != nil {
		_ = db.Close()
		return nil, err
	}
	h := &Handler{cfg: cfg, mux: http.NewServeMux(), engines: manager, store: db, httpEngine: httpAdapter, xunleiEngine: xunleiAdapter, aria2Engine: aria2Adapter}
	if err := h.restoreTasks(); err != nil {
		_ = manager.Close()
		_ = db.Close()
		return nil, err
	}
	h.routes()
	prefix := strings.TrimRight(cfg.GatewayPrefix, "/")
	if prefix == "" {
		h.handler = h.mux
	} else {
		h.handler = http.StripPrefix(prefix, h.mux)
	}
	return h, nil
}

func (h *Handler) restoreTasks() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tasks, err := h.store.List(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Engine == "aria2" && h.aria2Engine != nil {
			restored := h.aria2Engine.RestoreTask(task)
			if err := h.store.Upsert(ctx, restored); err != nil {
				return err
			}
		}
		if task.Engine == "xunlei" {
			restored := h.xunleiEngine.RestoreTask(task)
			if err := h.store.Upsert(ctx, restored); err != nil {
				return err
			}
		}
		if task.Engine == "http" {
			restored := h.httpEngine.RestoreTask(task)
			if err := h.store.Upsert(ctx, restored); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.handler.ServeHTTP(w, r) }
func (h *Handler) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var first error
	for _, name := range h.engines.Names() {
		e, _ := h.engines.Get(name)
		tasks, err := e.List(ctx)
		if err != nil {
			continue
		}
		for _, task := range tasks {
			if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
				if err := e.Pause(ctx, task.ID); err != nil && first == nil {
					first = err
				}
				task.Status = engine.StatusPaused
				task.SpeedBytes = 0
			}
			if err := h.store.Upsert(ctx, task); err != nil && first == nil {
				first = err
			}
		}
	}
	if err := h.engines.Close(); err != nil && first == nil {
		first = err
	}
	if err := h.store.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

func (h *Handler) routes() {
	h.settingsRoutes()
	h.xunleiRoutes()
	h.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "service": "fnsdownload", "version": "0.5.21", "runtime": "go", "database": "sqlite"})
	})
	h.mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"download_dir": filepath.Clean(h.cfg.DownloadDir), "data_dir": filepath.Clean(h.cfg.DataDir), "database": "sqlite"})
	})
	h.mux.HandleFunc("GET /api/engines", func(w http.ResponseWriter, r *http.Request) {
		type info struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
			Error string `json:"error,omitempty"`
		}
		out := make([]info, 0)
		for _, name := range h.engines.Names() {
			e, _ := h.engines.Get(name)
			err := e.Health(r.Context())
			item := info{Name: name, Ready: err == nil}
			if err != nil {
				item.Error = err.Error()
			}
			out = append(out, item)
		}
		writeJSON(w, 200, out)
	})
	h.mux.HandleFunc("GET /api/tasks", h.listTasks)
	h.mux.HandleFunc("POST /api/tasks", h.createTask)
	h.mux.HandleFunc("POST /api/tasks/{id}/{action}", h.taskAction)
	if info, err := os.Stat(filepath.Join(h.cfg.UIPath, "index.html")); err == nil && !info.IsDir() {
		h.mux.Handle("/", http.FileServer(http.Dir(h.cfg.UIPath)))
	} else {
		h.mux.Handle("/", webHandler())
	}
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayUser(w, r) {
		return
	}
	out := make(map[string]engine.Task)
	for _, name := range h.engines.Names() {
		e, _ := h.engines.Get(name)
		items, err := e.List(r.Context())
		if err != nil {
			continue
		}
		for _, task := range items {
			out[task.ID] = task
			_ = h.store.Upsert(r.Context(), task)
		}
	}
	stored, err := h.store.List(r.Context())
	if err == nil {
		for _, task := range stored {
			if _, ok := out[task.ID]; !ok {
				out[task.ID] = task
			}
		}
	}
	result := make([]engine.Task, 0, len(out))
	for _, task := range out {
		result = append(result, task)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	writeJSON(w, 200, result)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayUser(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input struct {
		Engine    string            `json:"engine"`
		URL       string            `json:"url"`
		Directory string            `json:"directory"`
		Name      string            `json:"name"`
		Headers   map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, 400, "invalid JSON body")
		return
	}
	if err := xunleiengine.ValidateHeaders(input.Headers); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if input.Engine == "" || input.Engine == "auto" {
		input.Engine = h.autoEngine(r.Context(), len(input.Headers) > 0)
		if input.Engine == "" {
			writeError(w, 503, "当前没有满足任务要求的可用下载引擎")
			return
		}
	}
	if len(input.Headers) > 0 && input.Engine != "xunlei" {
		writeError(w, 400, "自定义请求头目前仅适用于迅雷引擎")
		return
	}
	e, ok := h.engines.Get(input.Engine)
	if !ok {
		writeError(w, 400, "unknown download engine")
		return
	}
	dir, err := h.authorizedDirectory(r, input.Directory)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	task, err := e.Create(r.Context(), engine.CreateRequest{URL: input.URL, Directory: dir, Name: input.Name})
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err = h.store.Upsert(r.Context(), task); err != nil {
		_ = e.Delete(r.Context(), task.ID, false)
		writeError(w, 500, "could not persist download task")
		return
	}
	if len(input.Headers) > 0 {
		if err = e.(*xunleiengine.Engine).SetHeaders(r.Context(), task.ID, input.Headers); err != nil {
			_ = e.Delete(r.Context(), task.ID, false)
			_ = h.store.Delete(r.Context(), task.ID)
			writeError(w, 502, err.Error())
			return
		}
	}
	if err = e.Start(r.Context(), task.ID); err != nil {
		task.Status = engine.StatusFailed
		task.Error = err.Error()
		task.UpdatedAt = time.Now().UTC()
		_ = h.store.Upsert(r.Context(), task)
		writeError(w, 502, err.Error())
		return
	}
	if updated, getErr := e.Get(r.Context(), task.ID); getErr == nil {
		task = updated
	}
	_ = h.store.Upsert(r.Context(), task)
	writeJSON(w, 202, task)
}

func (h *Handler) taskAction(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayUser(w, r) {
		return
	}
	id, action := r.PathValue("id"), r.PathValue("action")
	var selected engine.Engine
	for _, name := range h.engines.Names() {
		e, _ := h.engines.Get(name)
		if _, err := e.Get(r.Context(), id); err == nil {
			selected = e
			break
		}
	}
	if selected == nil {
		writeError(w, 404, "task not found")
		return
	}
	var err error
	switch action {
	case "pause":
		err = selected.Pause(r.Context(), id)
	case "resume":
		err = selected.Resume(r.Context(), id)
	case "delete", "delete-files":
		err = selected.Delete(r.Context(), id, action == "delete-files")
	default:
		writeError(w, 400, "unsupported task action")
		return
	}
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	if action == "delete" || action == "delete-files" {
		if err := h.store.Delete(r.Context(), id); err != nil {
			writeError(w, 500, "could not remove task record")
			return
		}
	} else if task, getErr := selected.Get(r.Context(), id); getErr == nil {
		_ = h.store.Upsert(r.Context(), task)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) safeDownloadDir(relative string) (string, error) {
	root, err := filepath.Abs(h.cfg.DownloadDir)
	if err != nil {
		return "", err
	}
	target := root
	if relative != "" {
		if filepath.IsAbs(relative) {
			return "", errors.New("absolute download paths are not allowed")
		}
		target = filepath.Join(root, filepath.Clean(relative))
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("download path escapes the configured download directory")
	}
	if err := os.MkdirAll(target, 0750); err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	realRel, err := filepath.Rel(realRoot, realTarget)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
		return "", errors.New("download directory resolves outside the configured root")
	}
	return realTarget, nil
}
func requireGatewayUser(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("X-Trim-Userid")) == "" {
		writeError(w, 401, "request must come through the fnOS authenticated gateway")
		return false
	}
	return true
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
