package app

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type executionContract struct {
	Command          []string
	WorkingDirectory string
	Timeout          time.Duration
	OutputLimit      int
}

type executionOutputChunk struct {
	Stream string
	Chunk  string
}

type executionOutputEmitter func(executionOutputChunk) error

type executionRunOptions struct {
	StdoutStream string
	StderrStream string
	Emit         executionOutputEmitter
}

type executionRunResult struct {
	Output     []byte
	Truncated  bool
	ExitCode   *int
	DurationMS int64
	Backend    string
}

var runExecutionCommand = runLocalExecutionCommand

func localExecutionContract(workingDir string, command []string, timeout time.Duration, outputLimit int) executionContract {
	return executionContract{
		Command:          append([]string(nil), command...),
		WorkingDirectory: workingDir,
		Timeout:          timeout,
		OutputLimit:      outputLimit,
	}
}

func runLocalExecutionCommand(ctx context.Context, contract executionContract, opts executionRunOptions) (executionRunResult, error) {
	if len(contract.Command) == 0 || strings.TrimSpace(contract.Command[0]) == "" {
		return executionRunResult{}, errors.New("command is required")
	}
	if strings.TrimSpace(contract.WorkingDirectory) == "" {
		return executionRunResult{}, errors.New("working_directory is required")
	}
	stdoutStream, stderrStream := executionStreamNames(opts)
	runCtx, cancel := executionCommandContext(ctx, contract.Timeout)
	defer cancel()
	startedAt := time.Now()

	cmd := exec.CommandContext(runCtx, contract.Command[0], contract.Command[1:]...)
	cmd.Dir = contract.WorkingDirectory
	configureForegroundExecutionCommand(cmd)

	collector := newExecutionOutputCollector(contract.OutputLimit, opts.Emit)
	cmd.Stdout = collector.Writer(stdoutStream)
	cmd.Stderr = collector.Writer(stderrStream)

	if err := cmd.Start(); err != nil {
		return executionRunResult{}, err
	}
	stopTermination := watchForegroundExecutionCancellation(runCtx, cmd)
	err := cmd.Wait()
	stopTermination()
	return executionRunResult{
		Output:     collector.Bytes(),
		Truncated:  collector.Truncated(),
		ExitCode:   executionExitCodePointer(err),
		DurationMS: time.Since(startedAt).Milliseconds(),
		Backend:    "process",
	}, err
}

func executionCommandContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func executionStreamNames(opts executionRunOptions) (string, string) {
	stdoutStream := strings.TrimSpace(opts.StdoutStream)
	if stdoutStream == "" {
		stdoutStream = "stdout"
	}
	stderrStream := strings.TrimSpace(opts.StderrStream)
	if stderrStream == "" {
		stderrStream = "stderr"
	}
	return stdoutStream, stderrStream
}

func watchForegroundExecutionCancellation(ctx context.Context, cmd *exec.Cmd) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateForegroundExecutionCommand(cmd)
		case <-done:
		}
	}()
	return func() {
		close(done)
	}
}

type executionOutputCollector struct {
	mu        sync.Mutex
	limit     int
	buf       bytes.Buffer
	truncated bool
	emit      executionOutputEmitter
}

func newExecutionOutputCollector(limit int, emit executionOutputEmitter) *executionOutputCollector {
	return &executionOutputCollector{limit: limit, emit: emit}
}

func (c *executionOutputCollector) Writer(stream string) *executionOutputWriter {
	return &executionOutputWriter{stream: stream, collector: c}
}

func (c *executionOutputCollector) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

func (c *executionOutputCollector) Truncated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.truncated
}

type executionOutputWriter struct {
	stream    string
	collector *executionOutputCollector
}

func (w *executionOutputWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.collector.mu.Lock()
	defer w.collector.mu.Unlock()
	if w.collector.emit != nil {
		if err := w.collector.emit(executionOutputChunk{Stream: w.stream, Chunk: string(p)}); err != nil {
			return 0, err
		}
	}
	if w.collector.limit == 0 {
		_, _ = w.collector.buf.Write(p)
		return len(p), nil
	}
	remaining := w.collector.limit - w.collector.buf.Len()
	if remaining <= 0 {
		w.collector.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = w.collector.buf.Write(p[:remaining])
		w.collector.truncated = true
		return len(p), nil
	}
	_, _ = w.collector.buf.Write(p)
	return len(p), nil
}

func executionExitCodePointer(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}
