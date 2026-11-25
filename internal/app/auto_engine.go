package app

import (
	"context"
	"strings"
)

// Check availability on submission instead of relying on a stale browser snapshot.
// Headers require Xunlei; falling back must not silently drop them.
func (h *Handler) autoEngine(ctx context.Context, needsHeaders bool) string {
	for _, name := range []string{"xunlei", "aria2", "http"} {
		if needsHeaders && name != "xunlei" {
			continue
		}
		if e, ok := h.engines.Get(name); ok && e.Health(ctx) == nil {
			return name
		}
	}
	return ""
}

func (h *Handler) autoSourceEngine(ctx context.Context, source string, needsHeaders bool) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "magnet:") {
		if needsHeaders {
			return ""
		}
		if e, ok := h.engines.Get("qbittorrent"); ok && e.Health(ctx) == nil {
			return "qbittorrent"
		}
		return ""
	}
	return h.autoEngine(ctx, needsHeaders)
}
