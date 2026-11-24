//go:build !linux

package qbittorrent

import (
	"context"
	"errors"
)

func (e *Engine) startProcess(context.Context) (*managedProcess, error) {
	return nil, errors.New("独立 qBittorrent 仅支持 Linux amd64")
}
func (e *Engine) stopProcess() error { return nil }
