// Package logging provides a global verbose logging toggle for debug output.
package logging

import (
	"log"
	"sync/atomic"
)

var verbose atomic.Bool

// SetVerbose enables or disables verbose (hot-path) logging.
// Call once at startup based on the debug config flag.
func SetVerbose(v bool) { verbose.Store(v) }

// Verbose returns true when verbose logging is enabled.
func Verbose() bool { return verbose.Load() }

// Debugf logs only when verbose mode is enabled. Use for high-frequency
// events (per-token streaming, per-frame rendering, SSE publishing) that
// would otherwise dominate the log file.
func Debugf(format string, args ...any) {
	if verbose.Load() {
		log.Printf(format, args...)
	}
}
