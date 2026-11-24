package app

import (
	"context"
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"strings"
	"time"
)

type retryState struct {
	Count int
	Next  *time.Time
}

// Unknown SDK failures deliberately require manual intervention.
func transientDownloadFailure(task engine.Task) bool {
	if task.Status != engine.StatusFailed {
		return false
	}
	if task.Engine == "aria2" {
		var code int
		if _, err := fmt.Sscanf(task.Error, "aria2 任务错误 %d", &code); err != nil {
			return false
		}
		return code == 2 || code == 5 || code == 6 || code == 19 || code == 29
	}
	if task.Engine != "http" {
		return false
	}
	var code int
	if _, err := fmt.Sscanf(task.Error, "HTTP status %d", &code); err == nil {
		return code == 408 || code == 429 || code == 500 || code == 502 || code == 503 || code == 504
	}
	message := strings.ToLower(task.Error)
	for _, word := range []string{"token", "secret", "authorization", "certificate", "permission", "denied", "no space", "授权"} {
		if strings.Contains(message, word) {
			return false
		}
	}
	for _, word := range []string{"timeout", "timed out", "connection reset", "connection refused", "unexpected eof", "temporary failure in name resolution", "no such host"} {
		if strings.Contains(message, word) {
			return true
		}
	}
	return false
}

func (h *Handler) retryMetadata(task engine.Task) engine.Task {
	task.Retryable = !h.retryBlocked[task.ID] && transientDownloadFailure(task)
	task.RetryCount = 0
	task.RetryAt = nil
	if state, ok := h.retryStates[task.ID]; ok {
		task.RetryCount = state.Count
		task.RetryAt = state.Next
	}
	return task
}

// Called under queueMu; delays are per task and do not occupy download slots.
func (h *Handler) prepareAutoRetries(ctx context.Context, tasks []engine.Task, limit int, now time.Time) {
	if limit == 0 {
		h.retryStates = nil
		return
	}
	if h.retryStates == nil {
		h.retryStates = make(map[string]retryState)
	}
	present := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		present[task.ID] = true
		state := h.retryStates[task.ID]
		if h.retryBlocked[task.ID] {
			delete(h.retryStates, task.ID)
			continue
		}
		if _, waiting := h.queue[task.ID]; waiting {
			continue
		}
		if task.Status == engine.StatusCompleted || task.Status == engine.StatusRemoved {
			delete(h.retryStates, task.ID)
			continue
		}
		if !transientDownloadFailure(task) || state.Count >= limit {
			state.Next = nil
			h.retryStates[task.ID] = state
			continue
		}
		if state.Next == nil {
			next := now.Add(30 * time.Second * time.Duration(1<<state.Count))
			state.Next = &next
			h.retryStates[task.ID] = state
			continue
		}
		if now.Before(*state.Next) {
			continue
		}
		e, ok := h.engines.Get(task.Engine)
		if !ok {
			continue
		}
		state.Count++
		state.Next = nil
		if err := h.enqueue(ctx, e, task.ID); err != nil {
			if h.queueFailures == nil {
				h.queueFailures = make(map[string]string)
			}
			h.queueFailures[task.ID] = err.Error()
		}
		h.retryStates[task.ID] = state
	}
	for id := range h.retryStates {
		if !present[id] {
			delete(h.retryStates, id)
		}
	}
}
