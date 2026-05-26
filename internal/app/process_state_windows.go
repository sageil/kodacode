//go:build windows

package app

import (
	"errors"
	"os"
	"syscall"
)

func loadBackgroundProcessState(pid int) (backgroundProcessState, error) {
	if pid <= 0 {
		return backgroundProcessState{}, nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return backgroundProcessState{}, nil
	}
	err = process.Signal(syscall.Signal(0))
	if err != nil && !errors.Is(err, os.ErrPermission) {
		return backgroundProcessState{}, nil
	}
	return backgroundProcessState{Running: true}, nil
}
