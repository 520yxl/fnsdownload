package app

import (
	"context"
	"errors"
	"github.com/520yxl/fnsdownload/internal/engine"
	"testing"
	"time"
)

type availabilityEngine struct {
	engine.Engine
	name    string
	failure error
}

func (e *availabilityEngine) Name() string                 { return e.name }
func (e *availabilityEngine) Health(context.Context) error { return e.failure }
func TestAutomaticEngineSelection(t *testing.T) {
	manager := engine.NewManager()
	xl := &availabilityEngine{name: "xunlei"}
	http := &availabilityEngine{name: "http"}
	if err := manager.Register(xl); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(http); err != nil {
		t.Fatal(err)
	}
	h := &Handler{engines: manager}
	ctx := context.Background()
	if h.autoEngine(ctx, false) != "xunlei" {
		t.Fatal("Xunlei not preferred")
	}
	xl.failure = errors.New("unavailable")
	if h.autoEngine(ctx, false) != "http" {
		t.Fatal("HTTP fallback missing")
	}
	if h.autoEngine(ctx, true) != "" {
		t.Fatal("header requirement silently discarded")
	}
	http.failure = errors.New("unavailable")
	if h.autoEngine(ctx, false) != "" {
		t.Fatal("unavailable engine selected")
	}
}

func TestBTAutomaticSelectionAndNoHTTPFallback(t *testing.T) {
	m := engine.NewManager()
	qb := &availabilityEngine{name: "qbittorrent"}
	web := &availabilityEngine{name: "http"}
	_ = m.Register(qb)
	_ = m.Register(web)
	h := &Handler{engines: m}
	ctx := context.Background()
	magnet := "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if h.autoSourceEngine(ctx, magnet, false) != "qbittorrent" {
		t.Fatal("magnet not routed to BT")
	}
	if h.autoSourceEngine(ctx, "https://example.com/a", false) != "http" {
		t.Fatal("HTTP routed to BT")
	}
	qb.failure = errors.New("unavailable")
	if h.autoSourceEngine(ctx, magnet, false) != "" {
		t.Fatal("BT fell back to HTTP")
	}
	qb.failure = nil
	if h.autoSourceEngine(ctx, magnet, true) != "" {
		t.Fatal("BT dropped headers")
	}
	if h.shouldFallback(engine.Task{ID: "qb-test", Engine: "qbittorrent", AutoEngine: true, Status: engine.StatusRunning}, time.Now()) {
		t.Fatal("BT eligible for HTTP fallback")
	}
}
