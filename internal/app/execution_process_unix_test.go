//go:build !windows

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunLocalExecutionCommandCancellationKillsProcessTree(t *testing.T) {
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	command := "sh -c 'echo $$ > child.pid; exec sleep 30'"
	contract := localExecutionContract(
		root,
		shellExecArgs(defaultExecutionConfig(), command, false),
		0,
		executionCommandOutputCap,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runResult struct {
		result executionRunResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := runLocalExecutionCommand(ctx, contract, executionRunOptions{
			StdoutStream: "combined",
			StderrStream: "combined",
		})
		done <- runResult{result: result, err: err}
	}()

	pid := waitForForegroundChildPID(t, pidFile)
	cancel()

	var finished runResult
	select {
	case finished = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for canceled execution to finish")
	}
	if finished.err == nil {
		t.Fatal("expected execution cancellation error")
	}

	waitForForegroundProcessExit(t, pid)
}

func waitForForegroundChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for child pid file %q", path)
	return 0
}

func waitForForegroundProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stat, err := processStateForTest(pid)
		if err != nil || stat == "" || strings.HasPrefix(stat, "Z") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("process %d still running after cancellation", pid)
}

func processStateForTest(pid int) (string, error) {
	output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
