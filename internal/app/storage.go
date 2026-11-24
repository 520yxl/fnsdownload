package app

import (
	"fmt"
	"github.com/520yxl/fnsdownload/internal/engine"
	"math"
)

type volumeSpace struct {
	Available int64
	Volume    string
}

func (h *Handler) diskSpace(path string) (volumeSpace, error) {
	if h.spaceProbe != nil {
		return h.spaceProbe(path)
	}
	return readVolumeSpace(path)
}
func remainingBytes(task engine.Task) int64 {
	if task.TotalBytes <= 0 || task.CompletedBytes >= task.TotalBytes {
		return 0
	}
	if task.CompletedBytes < 0 {
		return task.TotalBytes
	}
	return task.TotalBytes - task.CompletedBytes
}
func spaceRequired(remaining, reserve int64) int64 {
	if remaining > math.MaxInt64-reserve {
		return math.MaxInt64
	}
	return remaining + reserve
}
func spaceDescription(available, required int64) string {
	return fmt.Sprintf("保存目录空间不足：可用 %.1f GB，预计至少需要 %.1f GB（含保留空间）", float64(available)/(1<<30), float64(required)/(1<<30))
}
func (h *Handler) storageMetadata(task engine.Task) engine.Task {
	if reason := h.spacePaused[task.ID]; reason != "" && task.Status != engine.StatusFailed && task.Status != engine.StatusCompleted {
		task.Error = reason
	}
	if reason := h.spaceWait[task.ID]; reason != "" {
		task.Error = reason
	}
	return task
}
