//go:build !darwin && !linux

package search

import "os"

type observedState struct{}

func observedStateForInfo(info os.FileInfo) (observedState, bool, error) {
	return observedState{}, false, nil
}

func observedStateToken(state observedState) (string, bool) {
	return "", false
}
