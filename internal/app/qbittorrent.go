package app

import (
	"encoding/json"
	qbengine "github.com/520yxl/fnsdownload/internal/engine/qbittorrent"
	"io"
	"net/http"
)

func (h *Handler) qbRoutes() {
	h.mux.HandleFunc("GET /api/qbittorrent/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		if h.qbEngine == nil {
			writeError(w, 503, "本应用尚未打包 qBittorrent 引擎")
			return
		}
		writeJSON(w, 200, map[string]any{"options": h.qbEngine.Options()})
	})
	h.mux.HandleFunc("PUT /api/qbittorrent/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireGatewayUser(w, r) {
			return
		}
		if h.qbEngine == nil {
			writeError(w, 503, "本应用尚未打包 qBittorrent 引擎")
			return
		}
		var options qbengine.Options
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&options) != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(w, 400, "无效的 qBittorrent 设置")
			return
		}
		if err := h.qbEngine.Configure(r.Context(), options); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, h.qbEngine.Options())
	})
}
