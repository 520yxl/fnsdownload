package app

import (
	"context"
	"errors"
	"github.com/520yxl/fnsdownload/internal/engine"
	"testing"
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
