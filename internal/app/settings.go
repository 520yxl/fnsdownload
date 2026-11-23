package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

type appSettings struct {
	Directory string `json:"directory"`
	Interval  int    `json:"interval"`
	Theme     string `json:"theme"`
}

func (h *Handler) readSettings() (appSettings, error) {
	value := appSettings{Interval: 2500, Theme: "auto"}
	data, err := os.ReadFile(filepath.Join(h.cfg.DataDir, "settings.json"))
	if os.IsNotExist(err) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if err = json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	return value, validateAppSettings(value)
}
func validateAppSettings(v appSettings) error {
	if v.Interval != 2500 && v.Interval != 5000 && v.Interval != 10000 {
		return errors.New("无效的同步间隔")
	}
	if v.Theme != "auto" && v.Theme != "light" && v.Theme != "dark" {
		return errors.New("无效的主题模式")
	}
	if v.Directory != "" && !filepath.IsAbs(v.Directory) {
		return errors.New("默认目录必须通过飞牛目录选择器选择")
	}
	return nil
}
func (h *Handler) settingsRoutes() {
	h.mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		h.settingsMu.Lock()
		defer h.settingsMu.Unlock()
		value, err := h.readSettings()
		if err != nil {
			writeError(w, 500, "无法读取应用设置")
			return
		}
		writeJSON(w, 200, value)
	})
	h.mux.HandleFunc("PATCH /api/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		var patch struct {
			Directory *string `json:"directory"`
			Interval  *int    `json:"interval"`
			Theme     *string `json:"theme"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&patch) != nil {
			writeError(w, 400, "无效的设置")
			return
		}
		h.settingsMu.Lock()
		defer h.settingsMu.Unlock()
		value, err := h.readSettings()
		if err != nil {
			writeError(w, 500, "无法读取应用设置")
			return
		}
		if patch.Directory != nil {
			if *patch.Directory != "" {
				if !filepath.IsAbs(*patch.Directory) {
					writeError(w, 400, "请选择飞牛目录")
					return
				}
				path, err := authorizedDownloadDir(r.Context(), r.Header.Get("X-Trim-Userid"), *patch.Directory)
				if err != nil {
					writeError(w, 400, err.Error())
					return
				}
				value.Directory = path
			} else {
				value.Directory = ""
			}
		}
		if patch.Interval != nil {
			value.Interval = *patch.Interval
		}
		if patch.Theme != nil {
			value.Theme = *patch.Theme
		}
		if err = validateAppSettings(value); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		data, _ := json.MarshalIndent(value, "", "  ")
		file, err := os.CreateTemp(h.cfg.DataDir, ".settings-*")
		if err != nil {
			writeError(w, 500, "无法保存应用设置")
			return
		}
		name := file.Name()
		defer os.Remove(name)
		if _, err = file.Write(data); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(name, filepath.Join(h.cfg.DataDir, "settings.json"))
		}
		if err != nil {
			writeError(w, 500, "无法保存应用设置")
			return
		}
		writeJSON(w, 200, value)
	})
}
