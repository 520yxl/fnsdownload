//go:build windows

package app

import (
	"golang.org/x/sys/windows"
	"math"
	"path/filepath"
)

func readVolumeSpace(path string) (volumeSpace, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return volumeSpace{}, err
	}
	pointer, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return volumeSpace{}, err
	}
	var available, total, free uint64
	if err = windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free); err != nil {
		return volumeSpace{}, err
	}
	if available > math.MaxInt64 {
		available = math.MaxInt64
	}
	return volumeSpace{Available: int64(available), Volume: filepath.VolumeName(absolute)}, nil
}
