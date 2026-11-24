package app

import (
	"encoding/json"
	"errors"
	"github.com/520yxl/fnsdownload/internal/engine"
	xunleiengine "github.com/520yxl/fnsdownload/internal/engine/xunlei"
	"github.com/520yxl/fnsdownload/internal/xunlei"
	"net/http"
	"path/filepath"
)

func (h *Handler) getXunleiEngine() (*xunleiengine.Engine, error) {
	e, ok := h.engines.Get("xunlei")
	if !ok {
		return nil, engine.ErrUnsupported
	}
	x, ok := e.(*xunleiengine.Engine)
	if !ok {
		return nil, engine.ErrUnsupported
	}
	return x, nil
}
func (h *Handler) authorizedDirectory(r *http.Request, path string) (string, error) {
	if path == "" {
		h.settingsMu.Lock()
		preferences, err := h.readSettings()
		h.settingsMu.Unlock()
		if err != nil {
			return "", errors.New("无法读取应用设置")
		}
		path = preferences.Directory
	}

	if filepath.IsAbs(path) {
		return authorizedDownloadDir(r.Context(), r.Header.Get("X-Trim-Userid"), path)
	}
	return h.safeDownloadDir(path)
}
func (h *Handler) xunleiRoutes() {
	h.mux.HandleFunc("GET /api/xunlei/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		e, err := h.getXunleiEngine()
		if err != nil {
			writeError(w, 503, err.Error())
			return
		}
		version, _ := xunlei.RuntimeVersion()
		writeJSON(w, 200, map[string]any{"options": e.Options(), "sdk_version": version, "built": xunlei.Built()})
	})
	h.mux.HandleFunc("PUT /api/xunlei/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		e, err := h.getXunleiEngine()
		if err != nil {
			writeError(w, 503, err.Error())
			return
		}
		var o xunlei.Options
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&o) != nil {
			writeError(w, 400, "无效的设置")
			return
		}
		if err = e.Configure(r.Context(), o); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, e.Options())
	})
	h.mux.HandleFunc("GET /api/xunlei/tasks/{id}/details", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		e, err := h.getXunleiEngine()
		if err != nil {
			writeError(w, 503, err.Error())
			return
		}
		id := r.PathValue("id")
		if task, getErr := h.store.Get(r.Context(), id); getErr == nil {
			if task.Engine != "xunlei" {
				writeError(w, 400, "当前任务未使用迅雷引擎")
				return
			}
			id = nativeTaskID(task)
		}
		info, err := e.Details(r.Context(), id)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, info)
	})
	h.mux.HandleFunc("POST /api/xunlei/batch", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		writeError(w, 400, "请使用独立任务接口批量添加链接")
	})
	h.mux.HandleFunc("POST /api/xunlei/recover", func(w http.ResponseWriter, r *http.Request) {
		h.queueMu.Lock()
		defer h.queueMu.Unlock()
		if !requireGatewayUser(w, r) {
			return
		}
		e, err := h.getXunleiEngine()
		if err != nil {
			writeError(w, 503, err.Error())
			return
		}
		tasks, err := e.Recover(r.Context(), func(dir string) error {
			if !filepath.IsAbs(dir) {
				return errors.New("invalid SDK directory")
			}
			// The app-owned default directory does not require a separate user grant.
			root, _ := filepath.Abs(h.cfg.DownloadDir)
			rel, err := filepath.Rel(root, filepath.Clean(dir))
			if err == nil && rel != ".." && !filepath.IsAbs(rel) && !(len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)) {
				return nil
			}
			_, err = h.authorizedDirectory(r, dir)
			return err
		})
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		savedTasks, listErr := h.store.List(r.Context())
		if listErr != nil {
			writeError(w, 500, "无法读取任务记录")
			return
		}
		bindings := make(map[string]engine.Task)
		for _, saved := range savedTasks {
			if saved.Engine == "xunlei" {
				bindings[nativeTaskID(saved)] = saved
			}
		}
		for _, task := range tasks {
			if saved, ok := bindings[task.ID]; ok {
				task = bindTask(task, saved)
			}
			if err = h.store.Upsert(r.Context(), task); err != nil {
				writeError(w, 500, "无法保存恢复的任务")
				return
			}
		}
		writeJSON(w, 200, map[string]any{"recovered": len(tasks)})
	})
}
