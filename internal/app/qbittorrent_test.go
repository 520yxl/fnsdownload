package app

import (
	"encoding/json"
	qbengine "github.com/520yxl/fnsdownload/internal/engine/qbittorrent"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQBittorrentSettingsAuthorizationValidationAndPersistence(t *testing.T) {
	root := t.TempDir()
	e := qbengine.NewManaged(qbengine.Config{StateDir: root})
	h := &Handler{mux: http.NewServeMux(), qbEngine: e}
	h.qbRoutes()
	request := func(method, body string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/qbittorrent/settings", strings.NewReader(body))
		if authorized {
			r.Header.Set("X-Trim-Userid", "1000")
		}
		w := httptest.NewRecorder()
		h.mux.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "", false); w.Code == 200 {
		t.Fatal("settings exposed without gateway identity")
	}
	desired := qbengine.DefaultOptions()
	desired.Connections = 4

	body, _ := json.Marshal(desired)
	if w := request("PUT", string(body), true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if reloaded := qbengine.NewManaged(qbengine.Config{StateDir: root}); reloaded.Options() != desired {
		t.Fatal("settings not persisted")
	}
	if w := request("PUT", `{"web_ui_address":"foreign","connections":4}`, true); w.Code != 400 {
		t.Fatal("RPC settings injection accepted", w.Code)
	}
	desired.Connections = 2001
	body, _ = json.Marshal(desired)
	if w := request("PUT", string(body), true); w.Code != 400 {
		t.Fatal("invalid connections accepted", w.Code)
	}
	if e.Options().Connections != 4 {
		t.Fatal("bad request changed prior setting")
	}
}
