package tool

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcAttr sets Unix process group isolation so the entire tree can be killed.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree sends SIGTERM to the process group of pid, waits 200 ms,
// then unconditionally sends SIGKILL.
func killTree(pid int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return err
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	return nil
}
