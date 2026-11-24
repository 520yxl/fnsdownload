package app

import (
	"encoding/json"
	aria2engine "github.com/520yxl/fnsdownload/internal/engine/aria2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAria2SettingsAuthorizationValidationAndPersistence(t *testing.T) {
	root := t.TempDir()
	e := aria2engine.NewManaged(aria2engine.Config{StateDir: root})
	h := &Handler{mux: http.NewServeMux(), aria2Engine: e}
	h.aria2Routes()
	request := func(method, body string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/aria2/settings", strings.NewReader(body))
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
	desired := aria2engine.DefaultOptions()
	desired.Connections = 4
	desired.Split = 4
	body, _ := json.Marshal(desired)
	if w := request("PUT", string(body), true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if reloaded := aria2engine.NewManaged(aria2engine.Config{StateDir: root}); reloaded.Options() != desired {
		t.Fatal("settings not persisted")
	}
	if w := request("PUT", `{"rpc-secret":"foreign","connections":4}`, true); w.Code != 400 {
		t.Fatal("RPC settings injection accepted", w.Code)
	}
	desired.Connections = 17
	body, _ = json.Marshal(desired)
	if w := request("PUT", string(body), true); w.Code != 400 {
		t.Fatal("invalid connections accepted", w.Code)
	}
	if e.Options().Connections != 4 {
		t.Fatal("bad request changed prior setting")
	}
}
