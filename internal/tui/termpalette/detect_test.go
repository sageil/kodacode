package termpalette_test

import (
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/tui/termpalette"
)

func TestDetect_StubResponse(t *testing.T) {
	// Build a fake terminal response: OSC responses for colors 0-15 + fg + bg.
	var buf strings.Builder
	for n := 0; n < 16; n++ {
		buf.WriteString("\x1b]4;" + strconv.Itoa(n) + ";rgb:0000/0000/0000\x07")
	}
	buf.WriteString("\x1b]10;rgb:cccc/cccc/cccc\x07") // fg
	buf.WriteString("\x1b]11;rgb:1a1a/1b1b/2626\x07") // bg → dark

	rw := &readWriter{r: strings.NewReader(buf.String())}
	p, err := termpalette.Detect(rw, io.Discard, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsDark {
		t.Error("expected IsDark=true for dark background")
	}
	if p.Fg == "" {
		t.Error("expected non-empty Fg")
	}
}

type readWriter struct{ r io.Reader }

func (rw *readWriter) Read(p []byte) (int, error)  { return rw.r.Read(p) }
func (rw *readWriter) Write(p []byte) (int, error) { return len(p), nil }
