// Package procsignal records the OS signal, if any, that initiated process
// shutdown.
//
// The signal handler runs on its own goroutine and cancels the root context;
// by the time the in-flight work has unwound and the entrypoint sees a
// context.Canceled, the signal that caused it is long gone. Recording it here
// gives the entrypoint a single source of truth for two questions it cannot
// answer from the error alone: were we signalled at all, and by which signal.
//
// Both answers matter. A context.Canceled that arose without a signal is a
// genuine failure and must be reported as one rather than masquerading as a
// user abort. And a SIGTERM from a supervisor must exit 143 rather than
// impersonating a Ctrl-C's 130.
package procsignal

import (
	"os"
	"sync/atomic"
)

// holder wraps the stored signal so atomic.Value always observes one concrete
// type. Storing differing concrete types (or nil) into an atomic.Value panics;
// wrapping avoids both.
type holder struct{ sig os.Signal }

var caught atomic.Value // stores holder

// Store records sig as the signal that initiated shutdown. Safe for concurrent
// use; last writer wins, which is fine because every caller stores a genuine
// terminating signal.
func Store(sig os.Signal) {
	caught.Store(holder{sig: sig})
}

// Load returns the recorded terminating signal, or nil if none was recorded.
func Load() os.Signal {
	if h, ok := caught.Load().(holder); ok {
		return h.sig
	}
	return nil
}

// Reset clears the recorded signal. It exists for tests that need a clean
// slate; production code never clears it, because the process is on its way
// out by the time anything is recorded.
func Reset() {
	caught.Store(holder{})
}
