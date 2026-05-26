package app

import (
	"errors"
	"strings"
)

var (
	errBackgroundProcessNotRunning          = errors.New("background process is not running")
	errBackgroundProcessIdentityUnavailable = errors.New("background process identity is unavailable")
)

type backgroundProcessState struct {
	Running  bool
	Identity string
}

var loadBackgroundProcessStateFunc = loadBackgroundProcessState

func captureBackgroundProcessIdentity(pid int) (string, error) {
	state, err := loadBackgroundProcessStateFunc(pid)
	if err != nil {
		return "", err
	}
	if !state.Running {
		return "", errBackgroundProcessNotRunning
	}
	identity := strings.TrimSpace(state.Identity)
	if identity == "" {
		return "", errBackgroundProcessIdentityUnavailable
	}
	return identity, nil
}
