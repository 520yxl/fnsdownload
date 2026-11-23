package app

import "context"

// Check availability on submission instead of relying on a stale browser snapshot.
// Headers require Xunlei; falling back must not silently drop them.
func (h *Handler) autoEngine(ctx context.Context, needsHeaders bool) string {
	for _, name := range []string{"xunlei", "http"} {
		if needsHeaders && name != "xunlei" {
			continue
		}
		if e, ok := h.engines.Get(name); ok && e.Health(ctx) == nil {
			return name
		}
	}
	return ""
}
