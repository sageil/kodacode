//go:build !darwin && !linux

package tool

import "os"

type observedVersionState struct{}

func observedVersionStateForInfo(info os.FileInfo) (observedVersionState, bool, error) {
	return observedVersionState{}, false, nil
}

func observedVersionStateToken(state observedVersionState) (string, bool) {
	return "", false
}
