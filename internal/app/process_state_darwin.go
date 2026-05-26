//go:build darwin

package app

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func loadBackgroundProcessState(pid int) (backgroundProcessState, error) {
	if pid <= 0 {
		return backgroundProcessState{}, nil
	}
	kinfo, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			return backgroundProcessState{}, nil
		}
		return backgroundProcessState{}, err
	}
	if kinfo == nil || int(kinfo.Proc.P_pid) != pid {
		return backgroundProcessState{}, nil
	}
	return backgroundProcessState{
		Running:  true,
		Identity: fmt.Sprintf("%d.%06d", kinfo.Proc.P_starttime.Sec, kinfo.Proc.P_starttime.Usec),
	}, nil
}
