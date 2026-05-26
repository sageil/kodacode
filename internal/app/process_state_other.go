//go:build !darwin && !linux && !windows

package app

func loadBackgroundProcessState(pid int) (backgroundProcessState, error) {
	return backgroundProcessState{}, nil
}
