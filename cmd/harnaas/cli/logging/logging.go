// Package logging writes harnaas's diagnostics to a file, and never to the
// terminal.
//
// # Why a file, and only a file
//
// The terminal belongs to the command's own output. A diagnostic on stdout
// would corrupt a --json document mid-parse; one on stderr would interleave
// with the advisory text a user is trying to read. So records go to a log file
// under the user's cache directory, and where that file cannot be opened they
// are discarded rather than falling back to a stream — a fallback that turns a
// disk problem into a parse failure is worse than no logging at all.
//
// The file lives outside the project on purpose. harnaas is a tool that writes
// into repositories, and a log directory appearing in a team's working tree
// after a read-only command would be a side effect nobody asked for.
//
// # What must never be logged
//
// Identifiers, paths, durations, counts and outcomes may be logged. File
// contents, instruction or rule text, captured output and credentials may not.
//
// This rule is sharper for harnaas than for most tools. The files it handles are
// a team's instructions and rules — the prose they write for their own AI
// harnesses — and that is exactly the content nobody expects to find copied into
// a log they did not know existed. "The file failed to parse" is a diagnostic;
// the line that failed to parse is user content.
//
// The API is attribute-only for the same reason: every logged value arrives as a
// named field, so reviewing what harnaas records is reviewing a list of keys
// rather than reading through interpolated message strings.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Environment variables read by this package. There is no configuration
// library and no manifest key for either: logging has to work before anything
// is loaded, and both of these are things a person sets for one run while
// working out what harnaas did.
const (
	// LevelEnvVar sets the minimum level recorded: debug, info, warn or error.
	// Unset or unrecognized means info.
	LevelEnvVar = "HARNAAS_LOG_LEVEL"

	// FileEnvVar overrides the log file path outright. It takes precedence over
	// the per-user default; an unset value means the default.
	FileEnvVar = "HARNAAS_LOG_FILE"
)

// dirName and fileName spell the default location under the user's cache
// directory: <cache>/harnaas/logs/harnaas.log.
const (
	dirName  = "harnaas"
	logsDir  = "logs"
	fileName = "harnaas.log"
)

var (
	// mu guards logger and file. It is held across a write so a concurrent
	// close cannot pull the file out from under one.
	mu sync.RWMutex

	// logger is where records go. It starts discarding, and goes back to
	// discarding whenever the file cannot be opened, so a log call is always
	// safe and never reaches a terminal.
	logger = discarding()

	// file is the open log file, if any.
	file *os.File
)

// discarding returns a logger that drops every record. It is a real logger
// rather than a nil check at each call site so the "not opened yet" path and
// the "could not open" path behave identically — and it is emphatically not
// slog.Default(), which writes to stderr.
func discarding() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// Open directs records to harnaas's log file and returns the function that
// closes it. Call it once, from the entrypoint, and defer the result.
//
// It reports nothing on failure and returns no error, because there is nowhere
// to report to: the terminal is the command's, and a warning printed there is
// the exact interleaving this package exists to prevent. A run whose log file
// could not be opened simply produces no log.
func Open() func() {
	if err := open(resolveFile()); err != nil {
		Close()
	}
	return Close
}

// open points the package logger at path, creating the containing directory and
// appending to any log already there. It is separated from Open so tests can
// see the failure Open deliberately swallows.
func open(path string) error {
	Close()

	if path == "" {
		return fmt.Errorf("no log file location: set %s to choose one", FileEnvVar)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create the log directory %s: %w", filepath.Dir(path), err)
	}

	// 0600: the log records which projects this user ran harnaas against, which
	// is nobody else's business on a shared machine.
	//nolint:gosec // G304: the path is harnaas's own log location, or the one the user named in HARNAAS_LOG_FILE for their own run.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open the log file %s: %w", path, err)
	}

	mu.Lock()
	defer mu.Unlock()
	file = f
	// Unbuffered: harnaas writes a handful of records per run, so there is
	// nothing to gain by batching them, and a buffer loses exactly the records
	// that matter when a run dies unexpectedly.
	logger = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level()}))
	return nil
}

// Close flushes and closes the log file, and sends subsequent records back to
// the discarding logger. It is safe to call more than once.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	logger = discarding()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

// resolveFile returns the log file path. Precedence, documented here at its
// only read site: HARNAAS_LOG_FILE, then <user cache dir>/harnaas/logs. An
// empty result means neither is available, and open turns that into the error
// it reports to nobody.
func resolveFile() string {
	if override := strings.TrimSpace(os.Getenv(FileEnvVar)); override != "" {
		return override
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, dirName, logsDir, fileName)
}

// level returns the minimum level to record. Precedence, documented here at its
// only read site: HARNAAS_LOG_LEVEL, then info. An unrecognized value is info
// too — refusing to run because a debugging aid was misspelled would be a poor
// trade — and the misspelling is itself recorded once the logger is up.
func level() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(LevelEnvVar))) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug records developer-facing detail: what harnaas considered and skipped.
func Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	record(ctx, slog.LevelDebug, msg, attrs)
}

// Info records the outcome of a step a user would recognize.
func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	record(ctx, slog.LevelInfo, msg, attrs)
}

// Warn records something harnaas worked around and would rather the user knew
// about. The user-facing half of that belongs on stderr; this is the record of
// it.
func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	record(ctx, slog.LevelWarn, msg, attrs)
}

// Error records a failure. The user-facing explanation is the entrypoint's job;
// this is what a maintainer reads afterwards.
func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	record(ctx, slog.LevelError, msg, attrs)
}

// record writes one entry, prefixing the attributes carried by ctx so component
// and command sort ahead of the call site's own fields.
//
// The read lock spans the write: Close must not be able to swap the logger
// halfway through one.
func record(ctx context.Context, lvl slog.Level, msg string, attrs []slog.Attr) {
	mu.RLock()
	defer mu.RUnlock()

	logger.LogAttrs(ctx, lvl, msg, append(contextAttrs(ctx), attrs...)...)
}
