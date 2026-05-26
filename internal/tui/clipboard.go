package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrClipboardUnavailable = errors.New("clipboard unavailable")

type clipboardWriter interface {
	WriteText(context.Context, string) error
}

type clipboardWriterFunc func(context.Context, string) error

func (fn clipboardWriterFunc) WriteText(ctx context.Context, text string) error {
	return fn(ctx, text)
}

type systemClipboardWriter struct{}

type clipboardCommand struct {
	name string
	args []string
}

var systemClipboardCommands = []clipboardCommand{
	{name: "pbcopy"},
	{name: "wl-copy"},
	{name: "xclip", args: []string{"-selection", "clipboard"}},
	{name: "xsel", args: []string{"--clipboard", "--input"}},
	{name: "clip"},
}

func (systemClipboardWriter) WriteText(ctx context.Context, text string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	foundBackend := false
	var lastErr error
	for _, spec := range systemClipboardCommands {
		path, err := exec.LookPath(spec.name)
		if err != nil {
			continue
		}
		foundBackend = true
		cmd := exec.CommandContext(ctx, path, spec.args...)
		cmd.Stdin = strings.NewReader(text)
		if output, err := cmd.CombinedOutput(); err != nil {
			lastErr = err
			if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
				lastErr = fmt.Errorf("%s", trimmed)
			}
			continue
		}
		return nil
	}
	if !foundBackend {
		return ErrClipboardUnavailable
	}
	if lastErr != nil {
		return fmt.Errorf("clipboard copy failed: %w", lastErr)
	}
	return ErrClipboardUnavailable
}
