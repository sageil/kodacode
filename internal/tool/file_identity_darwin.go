//go:build darwin

package tool

import (
	"os"
	"syscall"
)

func platformFileIdentity(info os.FileInfo) (uint64, uint64, int64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, 0, false
	}
	return uint64(stat.Dev), uint64(stat.Ino), stat.Ctimespec.Sec*1e9 + stat.Ctimespec.Nsec, true
}
