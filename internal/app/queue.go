package app

import (
	"context"
	"sort"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

func (h *Handler) startQueue() {
	ctx, cancel := context.WithCancel(context.Background())
	h.queueCancel = cancel
	h.queueDone = make(chan struct{})
	h.queue = make(map[string]engine.Engine)
	go func() {
		defer close(h.queueDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.queueMu.Lock()
				attempt, stop := context.WithTimeout(ctx, 10*time.Second)
				h.dispatchQueue(attempt)
				stop()
				h.queueMu.Unlock()
			}
		}
	}()
}

// Caller holds queueMu. Waiting tasks remain paused inside their engines.
func (h *Handler) enqueue(ctx context.Context, e engine.Engine, id string) error {
	task, err := e.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Status == engine.StatusRunning {
		return nil
	}
	if task.Status == engine.StatusCompleted || task.Status == engine.StatusRemoved {
		return engine.ErrUnsupported
	}
	if h.queue == nil {
		h.queue = make(map[string]engine.Engine)
	}
	task.Status = engine.StatusQueued
	task.Error = ""
	if err := h.store.Upsert(ctx, task); err != nil {
		return err
	}
	h.queue[id] = e
	delete(h.queueFailures, id)
	return nil
}

func (h *Handler) dispatchQueue(ctx context.Context) {
	h.settingsMu.Lock()
	settings, err := h.readSettings()
	h.settingsMu.Unlock()
	if err != nil {
		return
	}
	if len(h.queue) == 0 && settings.AutoRetry == 0 {
		h.retryStates = nil
		return
	}
	stored, err := h.store.List(ctx)
	if err != nil {
		return
	}
	if settings.AutoRetry > 0 {
		for i, task := range stored {
			if task.Status == engine.StatusCompleted || task.Status == engine.StatusRemoved || task.Status == engine.StatusPaused {
				continue
			}
			if _, waiting := h.queue[task.ID]; waiting {
				continue
			}
			if h.retryBlocked[task.ID] {
				continue
			}
			e, ok := h.engines.Get(task.Engine)
			if !ok {
				continue
			}
			if updated, getErr := e.Get(ctx, task.ID); getErr == nil {
				task = updated
			}
			if reason, failed := h.queueFailures[task.ID]; failed {
				task.Status = engine.StatusFailed
				task.Error = reason
			}
			if task.Status != stored[i].Status || task.Error != stored[i].Error {
				_ = h.store.Upsert(ctx, task)
			}
			stored[i] = task
		}
	}
	h.prepareAutoRetries(ctx, stored, settings.AutoRetry, time.Now().UTC())
	active := 0
	for _, task := range stored {
		if _, failed := h.queueFailures[task.ID]; failed {
			continue
		}
		if _, waiting := h.queue[task.ID]; waiting {
			continue
		}
		e, ok := h.engines.Get(task.Engine)
		if !ok {
			continue
		}
		updated, getErr := e.Get(ctx, task.ID)
		if getErr == nil {
			task = updated
		}
		if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
			active++
		}
	}
	sort.Slice(stored, func(i, j int) bool {
		if stored[i].CreatedAt.Equal(stored[j].CreatedAt) {
			return stored[i].ID < stored[j].ID
		}
		return stored[i].CreatedAt.Before(stored[j].CreatedAt)
	})
	for _, task := range stored {
		if active >= settings.MaxConcurrent || ctx.Err() != nil {
			return
		}
		e, waiting := h.queue[task.ID]
		if !waiting {
			continue
		}
		if err := e.Start(ctx, task.ID); err != nil {
			task.Status = engine.StatusFailed
			task.Error = err.Error()
			if h.queueFailures == nil {
				h.queueFailures = make(map[string]string)
			}
			h.queueFailures[task.ID] = task.Error
		} else {
			active++
			if updated, err := e.Get(ctx, task.ID); err == nil {
				task = updated
			}
		}
		task.UpdatedAt = time.Now().UTC()
		_ = h.store.Upsert(ctx, task)
		delete(h.queue, task.ID)
	}
}
