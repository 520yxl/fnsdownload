package app

import (
	"encoding/json"
	aria2engine "github.com/520yxl/fnsdownload/internal/engine/aria2"
	"net/http"
)

func (h *Handler) aria2Routes() {
	h.mux.HandleFunc("GET /api/aria2/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		if h.aria2Engine == nil {
			writeError(w, 503, "本应用尚未打包 aria2 引擎")
			return
		}
		writeJSON(w, 200, map[string]any{"options": h.aria2Engine.Options()})
	})
	h.mux.HandleFunc("PUT /api/aria2/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		if h.aria2Engine == nil {
			writeError(w, 503, "本应用尚未打包 aria2 引擎")
			return
		}
		var options aria2engine.Options
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&options) != nil {
			writeError(w, 400, "无效的 aria2 设置")
			return
		}
		if err := h.aria2Engine.Configure(r.Context(), options); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, h.aria2Engine.Options())
	})
}
