//go:build linux

package app

import (
	"fmt"
	"golang.org/x/sys/unix"
	"math"
)

func readVolumeSpace(path string) (volumeSpace, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return volumeSpace{}, err
	}
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return volumeSpace{}, err
	}
	if fs.Bsize <= 0 {
		return volumeSpace{}, fmt.Errorf("invalid filesystem block size")
	}
	blocks := uint64(fs.Bavail)
	size := uint64(fs.Bsize)
	available := int64(math.MaxInt64)
	if blocks <= uint64(math.MaxInt64)/size {
		available = int64(blocks * size)
	}
	return volumeSpace{Available: available, Volume: fmt.Sprintf("device-%d", st.Dev)}, nil
}
