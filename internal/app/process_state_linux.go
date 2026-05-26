//go:build linux

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadBackgroundProcessState(pid int) (backgroundProcessState, error) {
	if pid <= 0 {
		return backgroundProcessState{}, nil
	}
	statPath := filepath.Join("/proc", fmt.Sprintf("%d", pid), "stat")
	raw, err := os.ReadFile(statPath)
	if err != nil {
		if os.IsNotExist(err) {
			return backgroundProcessState{}, nil
		}
		return backgroundProcessState{}, err
	}
	text := strings.TrimSpace(string(raw))
	closing := strings.LastIndex(text, ")")
	if closing < 0 || closing+2 >= len(text) {
		return backgroundProcessState{}, fmt.Errorf("parse %s: malformed stat data", statPath)
	}
	fields := strings.Fields(text[closing+2:])
	if len(fields) <= 19 {
		return backgroundProcessState{}, fmt.Errorf("parse %s: expected at least 20 fields after comm, got %d", statPath, len(fields))
	}
	startTime := strings.TrimSpace(fields[19])
	if startTime == "" {
		return backgroundProcessState{}, fmt.Errorf("parse %s: missing start time", statPath)
	}
	bootID := ""
	if rawBootID, bootErr := os.ReadFile("/proc/sys/kernel/random/boot_id"); bootErr == nil {
		bootID = strings.TrimSpace(string(rawBootID))
	}
	identity := startTime
	if bootID != "" {
		identity = bootID + ":" + startTime
	}
	return backgroundProcessState{
		Running:  true,
		Identity: identity,
	}, nil
}
