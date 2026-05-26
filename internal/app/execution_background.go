package app

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const executionBackgroundStartupTimeout = 15 * time.Second

type executionBackgroundRunOptions struct {
	StdoutStream  string
	StderrStream  string
	Emit          executionOutputEmitter
	ReadyPatterns []string
	LogWriter     io.WriteCloser
}

type executionBackgroundHandle struct {
	PID             int
	ProcessIdentity string
	Ready           <-chan executionBackgroundReadyEvent
	Exited          <-chan executionBackgroundExitEvent
}

type executionBackgroundReadyEvent struct {
	Message string
	Port    int
}

type executionBackgroundExitEvent struct {
	RunResult executionRunResult
	Err       error
}

var startBackgroundExecutionCommand = startLocalBackgroundExecutionCommand

func startLocalBackgroundExecutionCommand(_ context.Context, contract executionContract, opts executionBackgroundRunOptions) (executionBackgroundHandle, error) {
	if len(contract.Command) == 0 || strings.TrimSpace(contract.Command[0]) == "" {
		return executionBackgroundHandle{}, errors.New("command is required")
	}
	if strings.TrimSpace(contract.WorkingDirectory) == "" {
		return executionBackgroundHandle{}, errors.New("working_directory is required")
	}
	stdoutStream, stderrStream := executionStreamNames(executionRunOptions{
		StdoutStream: opts.StdoutStream,
		StderrStream: opts.StderrStream,
	})
	startedAt := time.Now()

	cmd := exec.Command(contract.Command[0], contract.Command[1:]...)
	cmd.Dir = contract.WorkingDirectory

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return executionBackgroundHandle{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return executionBackgroundHandle{}, err
	}

	collector := newExecutionOutputCollector(contract.OutputLimit, nil)
	detector := newExecutionBackgroundReadyDetector(opts.ReadyPatterns)
	exitCh := make(chan executionBackgroundExitEvent, 1)

	if err := cmd.Start(); err != nil {
		return executionBackgroundHandle{}, err
	}
	processIdentity, err := captureBackgroundProcessIdentity(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if opts.LogWriter != nil {
			_ = opts.LogWriter.Close()
		}
		return executionBackgroundHandle{}, err
	}

	var pumps sync.WaitGroup
	pumps.Add(2)
	go pumpBackgroundExecutionOutput(stdoutPipe, stdoutStream, collector, detector, opts.Emit, opts.LogWriter, &pumps)
	go pumpBackgroundExecutionOutput(stderrPipe, stderrStream, collector, detector, opts.Emit, opts.LogWriter, &pumps)

	go func() {
		err := cmd.Wait()
		pumps.Wait()
		if opts.LogWriter != nil {
			_ = opts.LogWriter.Close()
		}
		exitCh <- executionBackgroundExitEvent{
			RunResult: executionRunResult{
				Output:     collector.Bytes(),
				Truncated:  collector.Truncated(),
				ExitCode:   executionExitCodePointer(err),
				DurationMS: time.Since(startedAt).Milliseconds(),
				Backend:    "background_process",
			},
			Err: err,
		}
		close(exitCh)
	}()

	return executionBackgroundHandle{
		PID:             cmd.Process.Pid,
		ProcessIdentity: processIdentity,
		Ready:           detector.Ready(),
		Exited:          exitCh,
	}, nil
}

func pumpBackgroundExecutionOutput(
	reader io.Reader,
	stream string,
	collector *executionOutputCollector,
	detector *executionBackgroundReadyDetector,
	emit executionOutputEmitter,
	logWriter io.Writer,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	if reader == nil || collector == nil {
		return
	}
	writer := collector.Writer(stream)
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			if emit != nil {
				_ = emit(executionOutputChunk{Stream: stream, Chunk: string(chunk)})
			}
			if detector != nil {
				detector.Observe(string(chunk))
			}
			if logWriter != nil {
				_, _ = logWriter.Write(chunk)
			}
			_, _ = writer.Write(chunk)
		}
		if err != nil {
			return
		}
	}
}

type executionBackgroundReadyDetector struct {
	patterns []string
	ready    chan executionBackgroundReadyEvent

	once   sync.Once
	mu     sync.Mutex
	buffer string
}

func newExecutionBackgroundReadyDetector(patterns []string) *executionBackgroundReadyDetector {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if trimmed := strings.ToLower(strings.TrimSpace(pattern)); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return &executionBackgroundReadyDetector{
		patterns: normalized,
		ready:    make(chan executionBackgroundReadyEvent, 1),
	}
}

func (d *executionBackgroundReadyDetector) Observe(chunk string) {
	if d == nil || len(d.patterns) == 0 || chunk == "" {
		return
	}
	d.mu.Lock()
	d.buffer += chunk
	lines := strings.Split(d.buffer, "\n")
	d.buffer = lines[len(lines)-1]
	complete := lines[:len(lines)-1]
	d.mu.Unlock()

	for _, line := range complete {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		for _, pattern := range d.patterns {
			if strings.Contains(lower, pattern) {
				d.once.Do(func() {
					d.ready <- executionBackgroundReadyEvent{
						Message: trimmed,
						Port:    extractBackgroundReadyPort(trimmed),
					}
				})
				return
			}
		}
	}
}

func (d *executionBackgroundReadyDetector) Ready() <-chan executionBackgroundReadyEvent {
	if d == nil || len(d.patterns) == 0 {
		return nil
	}
	return d.ready
}

func extractBackgroundReadyPort(line string) int {
	fields := strings.Fields(strings.ReplaceAll(line, ",", " "))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if idx := strings.LastIndex(field, ":"); idx >= 0 && idx < len(field)-1 {
			port, err := strconv.Atoi(strings.TrimRight(field[idx+1:], "/"))
			if err == nil && port > 0 && port <= 65535 {
				return port
			}
		}
	}
	return 0
}
