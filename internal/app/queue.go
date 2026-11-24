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
	if saved, err := h.store.Get(ctx, id); err == nil {
		if bound, ok := h.engineFor(saved); ok {
			e = bound
		}
	}
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
	delete(h.spaceWait, id)
	delete(h.spacePaused, id)
	return nil
}

func (h *Handler) dispatchQueue(ctx context.Context) {
	h.settingsMu.Lock()
	settings, err := h.readSettings()
	h.settingsMu.Unlock()
	if err != nil {
		return
	}
	stored, err := h.store.List(ctx)
	if err != nil {
		return
	}
	if h.spaceWait == nil {
		h.spaceWait = make(map[string]string)
	}
	if h.spacePaused == nil {
		h.spacePaused = make(map[string]string)
	}
	reserve := int64(settings.DiskReserveMB) * 1024 * 1024
	reserved := make(map[string]int64)
	active := 0
	// Refresh only live tasks (or failed tasks eligible for automatic retry).
	for i, task := range stored {
		if _, waiting := h.queue[task.ID]; waiting {
			continue
		}
		e, ok := h.engineFor(task)
		if !ok {
			continue
		}
		live := task.Status == engine.StatusRunning || task.Status == engine.StatusQueued
		if live || (settings.AutoRetry > 0 && task.Status == engine.StatusFailed && !h.retryBlocked[task.ID]) {
			if updated, getErr := e.Get(ctx, task.ID); getErr == nil {
				task = updated
			}
		}
		if reason, failed := h.queueFailures[task.ID]; failed {
			task.Status = engine.StatusFailed
			task.Error = reason
		}
		if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
			info, probeErr := h.diskSpace(task.Directory)
			if probeErr != nil {
				h.spacePaused[task.ID] = "无法读取保存目录的空间信息，请检查目录或存储卷；当前任务未自动暂停"
			} else if info.Available >= reserve {
				delete(h.spacePaused, task.ID)
			}
			if probeErr == nil && info.Available < reserve {
				reason := spaceDescription(info.Available, reserve) + "；已暂停，请释放空间后手动继续"
				if pauseErr := e.Pause(ctx, task.ID); pauseErr == nil {
					if updated, getErr := e.Get(ctx, task.ID); getErr == nil {
						task = updated
					}
					task.Status = engine.StatusPaused
					task.SpeedBytes = 0
					task.Error = reason
					h.spacePaused[task.ID] = reason
					if h.retryBlocked == nil {
						h.retryBlocked = make(map[string]bool)
					}
					h.retryBlocked[task.ID] = true
					delete(h.retryStates, task.ID)
				} else {
					task.Error = "保存目录空间不足，暂停失败，请手动暂停或检查应用日志"
					h.spacePaused[task.ID] = task.Error
				}
			}
			if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
				active++
				if probeErr == nil {
					reserved[info.Volume] = spaceRequired(reserved[info.Volume], remainingBytes(task))
				}
			}
		}
		if task.Status == engine.StatusCompleted || task.Status == engine.StatusRemoved {
			delete(h.spacePaused, task.ID)
		}
		if task.Status == engine.StatusRunning || stored[i].Status == engine.StatusRunning || task.Status != stored[i].Status || task.Error != stored[i].Error {
			_ = h.store.Upsert(ctx, task)
		}
		if h.shouldFallback(task, time.Now()) {
			if replacement, switched := h.fallbackTask(ctx, task); switched {
				if task.Status == engine.StatusRunning || task.Status == engine.StatusQueued {
					active--
				}
				task = replacement
			}
		}
		stored[i] = task
	}
	h.prepareAutoRetries(ctx, stored, settings.AutoRetry, time.Now().UTC())
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
		if current, getErr := e.Get(ctx, task.ID); getErr == nil {
			task = current
		}
		info, probeErr := h.diskSpace(task.Directory)
		if probeErr != nil {
			h.spaceWait[task.ID] = "无法读取保存目录的可用空间，请检查目录或存储卷是否可用"
			continue
		}
		required := spaceRequired(spaceRequired(reserved[info.Volume], remainingBytes(task)), reserve)
		if info.Available < required {
			h.spaceWait[task.ID] = spaceDescription(info.Available, required) + "；等待释放空间"
			continue
		}
		delete(h.spaceWait, task.ID)
		if err := e.Start(ctx, task.ID); err != nil {
			task.Status = engine.StatusFailed
			task.Error = err.Error()
			if h.queueFailures == nil {
				h.queueFailures = make(map[string]string)
			}
			h.queueFailures[task.ID] = task.Error
		} else {
			active++
			reserved[info.Volume] = spaceRequired(reserved[info.Volume], remainingBytes(task))
			if updated, getErr := e.Get(ctx, task.ID); getErr == nil {
				task = updated
			}
		}
		task.UpdatedAt = time.Now().UTC()
		_ = h.store.Upsert(ctx, task)
		delete(h.queue, task.ID)
	}
}
