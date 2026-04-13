// Package termpalette queries the terminal for its color palette via OSC
// escape sequences (OSC 4/10/11) and returns the results as a [Palette].
package termpalette

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// Palette holds the terminal's color palette queried via OSC escape sequences.
type Palette struct {
	Colors [16]string // hex strings "#rrggbb", empty string if not returned by terminal
	Fg     string     // default foreground, e.g. "#c0caf5", empty if not returned
	Bg     string     // default background, e.g. "#1a1b26", empty if not returned
	IsDark bool       // true if background luminance < 0.5
}

// Detect queries the terminal's color palette via OSC 4/10/11 escape sequences.
// It must be called before bubbletea/tcell takes over stdin.
// Returns an error if in is not a TTY, or if the terminal doesn't respond within timeout.
//
// in is io.ReadWriter so that in tests a bytes.Buffer with pre-built responses
// can be passed, so no real TTY is needed for tests.
func Detect(in io.ReadWriter, out io.Writer, timeout time.Duration) (Palette, error) {
	// Only set raw mode if in is a real TTY.
	if f, ok := in.(*os.File); ok {
		fd := int(f.Fd())
		if !term.IsTerminal(fd) {
			return Palette{IsDark: true}, fmt.Errorf("termpalette: not a TTY")
		}
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return Palette{IsDark: true}, fmt.Errorf("termpalette: make raw: %w", err)
		}
		defer term.Restore(fd, oldState) //nolint:errcheck
	}

	// Write all 18 OSC queries at once.
	var q strings.Builder
	for n := 0; n < 16; n++ {
		fmt.Fprintf(&q, "\x1b]4;%d;?\x07", n)
	}
	q.WriteString("\x1b]10;?\x07")
	q.WriteString("\x1b]11;?\x07")
	if _, err := io.WriteString(out, q.String()); err != nil {
		return Palette{IsDark: true}, fmt.Errorf("termpalette: write queries: %w", err)
	}

	// Read responses in a goroutine so we can apply a timeout.
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 4096)
		var acc []byte
		for countResponses(acc) < 18 {
			n, err := in.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
			}
			if err != nil {
				ch <- result{acc, err}
				return
			}
		}
		ch <- result{acc, nil}
	}()

	var raw []byte
	var readErr error
	select {
	case r := <-ch:
		raw, readErr = r.data, r.err
	case <-time.After(timeout):
		// Return what we have plus a timeout error.
		// The goroutine may still be blocked on Read; we cannot cancel it
		// cleanly without closing the fd, which is the caller's responsibility.
		raw = nil
		// Drain whatever arrived in the channel if the goroutine finishes
		// just after the timer fires.
		select {
		case r := <-ch:
			raw = r.data
		default:
		}
		p := parseResponses(raw)
		p.IsDark = isDark(p.Bg)
		return p, fmt.Errorf("termpalette: timeout waiting for responses")
	}

	p := parseResponses(raw)
	p.IsDark = isDark(p.Bg)
	if readErr == io.EOF {
		readErr = nil
	}
	return p, readErr
}

// countResponses counts how many complete OSC responses are present in buf.
// A response ends with BEL (\x07) or ST (\x1b\\).
func countResponses(buf []byte) int {
	count := 0
	i := 0
	for i < len(buf) {
		// Find start of OSC sequence.
		idx := bytes.Index(buf[i:], []byte("\x1b]"))
		if idx < 0 {
			break
		}
		start := i + idx + 2 // skip \x1b]
		rest := buf[start:]

		// Find BEL or ST terminator.
		belIdx := bytes.IndexByte(rest, '\x07')
		stIdx := bytes.Index(rest, []byte("\x1b\\"))
		end := -1
		if belIdx >= 0 && (stIdx < 0 || belIdx < stIdx) {
			end = start + belIdx + 1
		} else if stIdx >= 0 {
			end = start + stIdx + 2
		}
		if end < 0 {
			break
		}
		count++
		i = end
	}
	return count
}

// parseResponses parses raw terminal OSC 4/10/11 responses into a Palette.
func parseResponses(raw []byte) Palette {
	var p Palette
	rest := raw
	for len(rest) > 0 {
		// Find start of next OSC response.
		idx := bytes.Index(rest, []byte("\x1b]"))
		if idx < 0 {
			break
		}
		rest = rest[idx+2:]

		// Find terminator: BEL or ST.
		belIdx := bytes.IndexByte(rest, '\x07')
		stIdx := bytes.Index(rest, []byte("\x1b\\"))
		end := -1
		advance := 0
		if belIdx >= 0 && (stIdx < 0 || belIdx < stIdx) {
			end = belIdx
			advance = end + 1
		} else if stIdx >= 0 {
			end = stIdx
			advance = end + 2
		}
		if end < 0 {
			break
		}
		body := string(rest[:end])
		rest = rest[advance:]

		parseOSCBody(body, &p)
	}
	return p
}

// parseOSCBody interprets a single OSC body (the part between \x1b] and the
// terminator) and writes the result into p.
func parseOSCBody(body string, p *Palette) {
	switch {
	case strings.HasPrefix(body, "4;"):
		// OSC 4 palette response: "4;N;rgb:RRRR/GGGG/BBBB" or "4;N;#RRGGBB"
		rest := body[2:]
		semi := strings.IndexByte(rest, ';')
		if semi < 0 {
			return
		}
		nStr := rest[:semi]
		colorStr := rest[semi+1:]
		n, err := strconv.Atoi(nStr)
		if err != nil || n < 0 || n > 15 {
			return
		}
		if hex, ok := parseColorValue(colorStr); ok {
			p.Colors[n] = hex
		}
	case strings.HasPrefix(body, "10;"):
		// OSC 10 foreground response.
		if hex, ok := parseColorValue(body[3:]); ok {
			p.Fg = hex
		}
	case strings.HasPrefix(body, "11;"):
		// OSC 11 background response.
		if hex, ok := parseColorValue(body[3:]); ok {
			p.Bg = hex
		}
	}
}

// parseColorValue parses a color value in either "rgb:RRRR/GGGG/BBBB" or
// "#RRGGBB" format and returns a lowercase "#rrggbb" hex string.
func parseColorValue(s string) (string, bool) {
	if strings.HasPrefix(s, "rgb:") {
		return parseRGBColon(s[4:])
	}
	if strings.HasPrefix(s, "#") {
		return parseHashHex(s)
	}
	return "", false
}

// parseRGBColon parses "RRRR/GGGG/BBBB" (16-bit per channel, take high byte).
func parseRGBColon(s string) (string, bool) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return "", false
	}
	var channels [3]uint8
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			return "", false
		}
		v, err := strconv.ParseUint(part, 16, 64)
		if err != nil {
			return "", false
		}
		// Take the high byte regardless of how many hex digits are provided.
		switch len(part) {
		case 1:
			channels[i] = uint8(v << 4)
		case 2:
			channels[i] = uint8(v)
		case 3:
			channels[i] = uint8(v >> 4)
		default: // 4 digits (standard 16-bit)
			channels[i] = uint8(v >> 8)
		}
	}
	return fmt.Sprintf("#%02x%02x%02x", channels[0], channels[1], channels[2]), true
}

// parseHashHex parses "#RRGGBB" and normalises to lowercase.
func parseHashHex(s string) (string, bool) {
	if len(s) != 7 {
		return "", false
	}
	r, err1 := strconv.ParseUint(s[1:3], 16, 8)
	g, err2 := strconv.ParseUint(s[3:5], 16, 8)
	b, err3 := strconv.ParseUint(s[5:7], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return "", false
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b), true
}

// isDark reports whether the hex background colour has a perceived luminance
// below 0.5 using the standard formula 0.299R + 0.587G + 0.114B.
// Returns true (dark) when bg is empty.
func isDark(bg string) bool {
	if bg == "" {
		return true
	}
	if len(bg) != 7 || bg[0] != '#' {
		return true
	}
	rv, err1 := strconv.ParseUint(bg[1:3], 16, 8)
	gv, err2 := strconv.ParseUint(bg[3:5], 16, 8)
	bv, err3 := strconv.ParseUint(bg[5:7], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return true
	}
	r := float64(rv) / 255.0
	g := float64(gv) / 255.0
	b := float64(bv) / 255.0
	lum := 0.299*r + 0.587*g + 0.114*b
	return lum < 0.5
}
