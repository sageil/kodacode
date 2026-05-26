package mcp

import (
	"bufio"
	"bytes"
	"errors"
	"testing"
)

func TestReadRPCFrameReturnsDelimitedResponse(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}` + "\nextra")))

	line, err := readRPCFrame(reader)
	if err != nil {
		t.Fatalf("readRPCFrame() error = %v", err)
	}
	if got := string(line); got != `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`+"\n" {
		t.Fatalf("line = %q", got)
	}
}

func TestReadRPCFrameRejectsOversizedResponse(t *testing.T) {
	oversized := append(bytes.Repeat([]byte("x"), stdioMaxFrameBytes), '\n')
	reader := bufio.NewReader(bytes.NewReader(oversized))

	line, err := readRPCFrame(reader)
	if !errors.Is(err, errMCPResponseTooLarge) {
		t.Fatalf("readRPCFrame() error = %v, want %v", err, errMCPResponseTooLarge)
	}
	if len(line) != 0 {
		t.Fatalf("line length = %d, want 0", len(line))
	}
}
