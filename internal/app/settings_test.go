package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationSettingsPersistAndPatch(t *testing.T) {
	dir := t.TempDir()
	makeHandler := func() *Handler {
		h := &Handler{cfg: Config{DataDir: dir}, mux: http.NewServeMux()}
		h.settingsRoutes()
		return h
	}
	h := makeHandler()
	request := func(handler *Handler, method, body, uid string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/settings", strings.NewReader(body))
		if uid != "" {
			r.Header.Set("X-Trim-Userid", uid)
		}
		w := httptest.NewRecorder()
		handler.mux.ServeHTTP(w, r)
		return w
	}
	if w := request(h, "GET", "", ""); w.Code != 401 && w.Code != 403 {
		t.Fatal("unauthenticated settings accessible")
	}
	if w := request(h, "PATCH", `{"interval":5000,"max_concurrent":3}`, "1000"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(h, "PATCH", `{"theme":"dark"}`, "1000"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	reloaded := makeHandler()
	w := request(reloaded, "GET", "", "1000")
	var value appSettings
	if json.Unmarshal(w.Body.Bytes(), &value) != nil || value.Theme != "dark" || value.Interval != 5000 || value.MaxConcurrent != 3 {
		t.Fatal("settings lost across handler restart or partial patch")
	}
	for _, body := range []string{`{"max_concurrent":0}`, `{"max_concurrent":101}`, `{"interval":1}`, `{"theme":"invalid"}`, `{"directory":"../outside"}`} {
		if w := request(reloaded, "PATCH", body, "1000"); w.Code != 400 {
			t.Fatal("invalid setting accepted", body, w.Code)
		}
	}
	w = request(reloaded, "GET", "", "1000")
	if json.Unmarshal(w.Body.Bytes(), &value) != nil || value.Theme != "dark" || value.Interval != 5000 || value.MaxConcurrent != 3 {
		t.Fatal("invalid update changed settings")
	}
}
