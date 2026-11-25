package qbittorrent

import (
	"os"
	"os/exec"
)

type managedProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	lock *os.File
}
