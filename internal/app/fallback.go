package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

// Stable application IDs survive switching between engine-specific native IDs.
type boundEngine struct {
	engine.Engine
	saved engine.Task
}

func bindTask(raw, saved engine.Task) engine.Task {
	raw.ID = saved.ID
	raw.NativeID = saved.NativeID
	raw.CreatedAt = saved.CreatedAt
	raw.ElapsedMS = saved.ElapsedMS
	raw.AutoEngine = saved.AutoEngine
	raw.HeadersRequired = saved.HeadersRequired
	raw.TriedEngines = saved.TriedEngines
	raw.FallbackRoot = saved.FallbackRoot
	raw.EngineNote = saved.EngineNote
	raw.RetiredNativeIDs = saved.RetiredNativeIDs
	return raw
}
func nativeTaskID(task engine.Task) string {
	if task.NativeID != "" {
		return task.NativeID
	}
	return task.ID
}
func (b *boundEngine) Get(ctx context.Context, id string) (engine.Task, error) {
	if id != b.saved.ID {
		return engine.Task{}, engine.ErrNotFound
	}
	task, err := b.Engine.Get(ctx, nativeTaskID(b.saved))
	return bindTask(task, b.saved), err
}
func (b *boundEngine) Start(ctx context.Context, id string) error {
	if id != b.saved.ID {
		return engine.ErrNotFound
	}
	return b.Engine.Start(ctx, nativeTaskID(b.saved))
}
func (b *boundEngine) Resume(ctx context.Context, id string) error { return b.Start(ctx, id) }
func (b *boundEngine) Pause(ctx context.Context, id string) error {
	if id != b.saved.ID {
		return engine.ErrNotFound
	}
	return b.Engine.Pause(ctx, nativeTaskID(b.saved))
}
func (b *boundEngine) Delete(ctx context.Context, id string, files bool) error {
	if id != b.saved.ID {
		return engine.ErrNotFound
	}
	return b.Engine.Delete(ctx, nativeTaskID(b.saved), files)
}
func (h *Handler) engineFor(task engine.Task) (engine.Engine, bool) {
	e, ok := h.engines.Get(task.Engine)
	if !ok {
		return nil, false
	}
	return &boundEngine{Engine: e, saved: task}, true
}

type progressWatch struct {
	Bytes int64
	Since time.Time
}

const stalledDownloadTimeout = 2 * time.Minute

// Sampling under queueMu also works when the browser is closed.
func (h *Handler) shouldFallback(task engine.Task, now time.Time) bool {
	if task.Engine == "qbittorrent" || !task.AutoEngine || task.HeadersRequired || h.retryBlocked[task.ID] || h.spacePaused[task.ID] != "" || h.spaceWait[task.ID] != "" {
		delete(h.progressWatch, task.ID)
		return false
	}
	if task.Status == engine.StatusFailed {
		delete(h.progressWatch, task.ID)
		return transientDownloadFailure(task)
	}
	if task.Status != engine.StatusRunning {
		delete(h.progressWatch, task.ID)
		return false
	}
	if h.progressWatch == nil {
		h.progressWatch = make(map[string]progressWatch)
	}
	previous, ok := h.progressWatch[task.ID]
	if !ok || previous.Bytes != task.CompletedBytes {
		h.progressWatch[task.ID] = progressWatch{Bytes: task.CompletedBytes, Since: now}
		return false
	}
	return now.Sub(previous.Since) >= stalledDownloadTimeout
}

// Each engine is attempted once. A fresh private directory prevents mixing
// incompatible partial-file formats or overwriting an existing download.
func (h *Handler) fallbackTask(ctx context.Context, task engine.Task) (engine.Task, bool) {
	tried := append([]string(nil), task.TriedEngines...)
	if !slices.Contains(tried, task.Engine) {
		tried = append(tried, task.Engine)
	}
	old, ok := h.engineFor(task)
	if !ok {
		return task, false
	}
	for _, name := range []string{"xunlei", "aria2", "http"} {
		if slices.Contains(tried, name) {
			continue
		}
		next, ok := h.engines.Get(name)
		if !ok || next.Health(ctx) != nil {
			continue
		}
		root := task.FallbackRoot
		if root == "" {
			root = task.Directory
		}
		if !filepath.IsAbs(root) {
			return task, false
		}
		dir, err := os.MkdirTemp(root, "MoxYi-fallback-")
		if err != nil {
			return task, false
		}
		tried = append(tried, name)
		task.TriedEngines = tried
		if err := h.store.Upsert(ctx, task); err != nil {
			_ = os.Remove(dir)
			return task, false
		}
		replacement, err := next.Create(ctx, engine.CreateRequest{URL: task.Source, Directory: dir, Name: task.Name})
		if err != nil {
			_ = os.Remove(dir)
			continue
		}
		// Never run both transfers, including stalled downloads that have no error.
		if task.Status == engine.StatusRunning {
			if err = old.Pause(ctx, task.ID); err != nil {
				_ = next.Delete(ctx, replacement.ID, false)
				_ = os.Remove(dir)
				return task, false
			}
		}
		before := task.Engine
		native := replacement.ID
		replacement = bindTask(replacement, task)
		replacement.NativeID = native
		replacement.RetiredNativeIDs = append(append([]string(nil), task.RetiredNativeIDs...), task.Engine+"/"+nativeTaskID(task))
		replacement.Status = engine.StatusQueued
		replacement.SpeedBytes = 0
		replacement.Error = ""
		replacement.TriedEngines = append([]string(nil), tried...)
		reason := task.Error
		if reason == "" {
			reason = "连续 2 分钟无下载进展"
		}
		replacement.EngineNote = fmt.Sprintf("%s → %s：%s；使用独立目录重新下载，原有文件保留", before, name, reason)
		if _, err = h.store.Save(ctx, replacement); err != nil {
			_ = next.Delete(ctx, native, false)
			_ = os.Remove(dir)
			return task, false
		}
		// Cleanup only this app's old task. If removal fails, it remains stopped;
		// unbound native records are not displayed or started by the queue.
		_ = old.Delete(ctx, task.ID, false)
		if h.queue == nil {
			h.queue = make(map[string]engine.Engine)
		}
		h.queue[task.ID] = &boundEngine{Engine: next, saved: replacement}
		delete(h.queueFailures, task.ID)
		delete(h.retryStates, task.ID)
		delete(h.spacePaused, task.ID)
		delete(h.spaceWait, task.ID)
		delete(h.progressWatch, task.ID)
		return replacement, true
	}
	return task, false
}
