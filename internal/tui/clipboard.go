package tui

import (
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type clipboardPastedMsg struct{ text string }

func pasteClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		return clipboardPastedMsg{text: readClipboard()}
	}
}

func writeClipboard(text string) {
	name, args := clipboardCopyCmd()
	if name == "" {
		return
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}

func clipboardCopyCmd() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard"}
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--input"}
		}
	}
	return "", nil
}

func readClipboard() string {
	name, args := clipboardPasteCmd()
	if name == "" {
		return ""
	}
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func clipboardPasteCmd() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "pbpaste", nil
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard", "-o"}
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--output"}
		}
	}
	return "", nil
}
