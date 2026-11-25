//go:build !linux

package aria2

import (
	"context"
	"errors"
)

func (e *Engine) startProcess(context.Context) (*managedProcess, error) {
	return nil, errors.New("独立 aria2 仅支持 Linux")
}
func (e *Engine) stopProcess() error { return nil }
