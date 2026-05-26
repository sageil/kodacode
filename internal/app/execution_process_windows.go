//go:build windows

package app

import "os/exec"

func configureForegroundExecutionCommand(cmd *exec.Cmd) {}

func terminateForegroundExecutionCommand(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
