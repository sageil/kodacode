//go:build darwin

package tool

import (
	"fmt"
	"os"
	"syscall"
)

type observedVersionState struct {
	size         int64
	modTimeNS    int64
	changeTimeNS int64
	device       uint64
	inode        uint64
	mode         uint32
	links        uint64
}

func observedVersionStateForInfo(info os.FileInfo) (observedVersionState, bool, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return observedVersionState{}, false, nil
	}
	return observedVersionState{
		size:         info.Size(),
		modTimeNS:    info.ModTime().UnixNano(),
		changeTimeNS: stat.Ctimespec.Sec*1_000_000_000 + stat.Ctimespec.Nsec,
		device:       uint64(stat.Dev),
		inode:        stat.Ino,
		mode:         uint32(info.Mode()),
		links:        uint64(stat.Nlink),
	}, true, nil
}

func observedVersionStateToken(state observedVersionState) (string, bool) {
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d", state.size, state.modTimeNS, state.changeTimeNS, state.device, state.inode, state.mode, state.links), true
}
