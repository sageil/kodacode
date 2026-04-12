//go:build !darwin && !linux

package tool

import "os"

func platformFileIdentity(info os.FileInfo) (uint64, uint64, int64, bool) {
	return 0, 0, 0, false
}
