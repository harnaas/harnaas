package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// logTo points the package at a fresh log file under t.TempDir and returns its
// path. Every test that logs goes through it, so no test can touch the real
// per-user log.
func logTo(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "harnaas.log")
	t.Setenv(FileEnvVar, path)
	require.NoError(t, open(resolveFile()))
	t.Cleanup(Close)
	return path
}

// records reads the log file back as one map per record.
func records(t *testing.T, path string) []map[string]any {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var out []map[string]any
	for line := range strings.Lines(strings.TrimSpace(string(raw))) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "log line is not JSON: %s", line)
		out = append(out, rec)
	}
	return out
}

// silenceDefault redirects the process streams and slog's default logger into
// buffers, and asserts on cleanup that nothing reached any of them. It is how
// "logging never touches the terminal" is checked rather than assumed: the
// tempting fallback — writing to stderr when the log file cannot be opened — is
// exactly what would corrupt a --json document.
func silenceDefault(t *testing.T) {
	t.Helper()

	stdout, stderr := redirect(t, &os.Stdout), redirect(t, &os.Stderr)

	var fallback bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&fallback, nil)))

	t.Cleanup(func() {
		slog.SetDefault(previous)

		require.Empty(t, read(t, stdout), "logging wrote to stdout")
		require.Empty(t, read(t, stderr), "logging wrote to stderr")
		require.Empty(t, fallback.String(), "logging fell back to slog's default logger")
	})
}

// redirect replaces a process stream with a file under t.TempDir and returns its
// path, restoring the original on cleanup.
func redirect(t *testing.T, stream **os.File) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stream")
	f, err := os.Create(path)
	require.NoError(t, err)

	previous := *stream
	*stream = f
	t.Cleanup(func() {
		*stream = previous
		require.NoError(t, f.Close())
	})
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}

func TestRecordsGoToTheLogFileAndNowhereElse(t *testing.T) {
	silenceDefault(t)
	path := logTo(t)

	Info(t.Context(), "manifest loaded", slog.Int("assets", 3))

	got := records(t, path)
	require.Len(t, got, 1)
	require.Equal(t, "manifest loaded", got[0]["msg"])
	require.Equal(t, "INFO", got[0]["level"])
	require.InDelta(t, 3.0, got[0]["assets"], 0)
	require.Contains(t, got[0], "time")
}

func TestUnopenedLoggingDiscardsInsteadOfPrinting(t *testing.T) {
	silenceDefault(t)
	Close() // the state a binary is in before Open runs, and after it fails

	Info(t.Context(), "before the log file exists", slog.String("path", "harnaas.json"))
	Error(t.Context(), "and after it is closed again")
	// silenceDefault's cleanup asserts the streams stayed empty.
}

func TestContextLabelsEveryRecordWithCommandAndComponent(t *testing.T) {
	silenceDefault(t)
	path := logTo(t)

	ctx := WithComponent(WithCommand(t.Context(), "install"), "manifest")
	Warn(ctx, "source ref is a moving target", slog.String("source", "acme"))

	got := records(t, path)
	require.Len(t, got, 1)
	require.Equal(t, "install", got[0][CommandKey])
	require.Equal(t, "manifest", got[0][ComponentKey])
	require.Equal(t, "acme", got[0]["source"])
}

func TestAnInnerComponentReplacesTheOuterOneForItsOwnRecords(t *testing.T) {
	silenceDefault(t)
	path := logTo(t)

	outer := WithComponent(t.Context(), "install")
	inner := WithComponent(outer, "manifest")

	Info(inner, "inner")
	Info(outer, "outer")

	got := records(t, path)
	require.Len(t, got, 2)
	require.Equal(t, "manifest", got[0][ComponentKey])
	require.Equal(t, "install", got[1][ComponentKey], "the inner label does not leak back out")
}

func TestLevelFiltersRecordsBelowTheConfiguredThreshold(t *testing.T) {
	silenceDefault(t)
	t.Setenv(LevelEnvVar, "warn")
	path := logTo(t)

	Debug(t.Context(), "debug")
	Info(t.Context(), "info")
	Warn(t.Context(), "warn")
	Error(t.Context(), "error")

	got := records(t, path)
	require.Len(t, got, 2)
	require.Equal(t, "warn", got[0]["msg"])
	require.Equal(t, "error", got[1]["msg"])
}

func TestLevelDefaultsToInfoForEveryUnusableValue(t *testing.T) {
	for _, value := range []string{"", "   ", "verbose", "TRACE", "42"} {
		t.Run("level="+value, func(t *testing.T) {
			t.Setenv(LevelEnvVar, value)
			require.Equal(t, slog.LevelInfo, level(),
				"a misspelled debugging aid must not change what harnaas records")
		})
	}
}

func TestLevelAcceptsItsNamesCaseInsensitively(t *testing.T) {
	cases := []struct {
		value string
		want  slog.Level
	}{
		{value: "debug", want: slog.LevelDebug},
		{value: "DEBUG", want: slog.LevelDebug},
		{value: " Info ", want: slog.LevelInfo},
		{value: "warn", want: slog.LevelWarn},
		{value: "WARNING", want: slog.LevelWarn},
		{value: "Error", want: slog.LevelError},
	}

	for _, tc := range cases {
		t.Run("level="+tc.value, func(t *testing.T) {
			t.Setenv(LevelEnvVar, tc.value)
			require.Equal(t, tc.want, level())
		})
	}
}

func TestRecordsAppendToAnExistingLogRatherThanReplacingIt(t *testing.T) {
	silenceDefault(t)
	path := logTo(t)

	Info(t.Context(), "first run")
	Close()

	require.NoError(t, open(resolveFile()))
	Info(t.Context(), "second run")

	got := records(t, path)
	require.Len(t, got, 2, "a later run must not erase the run that produced the problem")
	require.Equal(t, "first run", got[0]["msg"])
	require.Equal(t, "second run", got[1]["msg"])
}

func TestTheDefaultLocationIsOutsideTheProject(t *testing.T) {
	t.Setenv(FileEnvVar, "")

	cache, err := os.UserCacheDir()
	require.NoError(t, err)

	// The project tree stays untouched by a read-only command, so the log lives
	// under the user's cache directory, not under the repository.
	require.Equal(t, filepath.Join(cache, dirName, logsDir, fileName), resolveFile())
}

func TestTheFileOverrideWins(t *testing.T) {
	chosen := filepath.Join(t.TempDir(), "elsewhere.log")
	t.Setenv(FileEnvVar, chosen)

	require.Equal(t, chosen, resolveFile())
}

func TestAnUnopenableLogFileLeavesTheCommandRunnable(t *testing.T) {
	silenceDefault(t)

	// A regular file where the log directory should be: MkdirAll fails on every
	// platform, which is the closest stand-in for a full or read-only disk.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	t.Setenv(FileEnvVar, filepath.Join(blocker, "logs", "harnaas.log"))

	require.Error(t, open(resolveFile()), "the failure is visible to this package")

	closeLog := Open()
	defer closeLog()

	// Open reports nothing, and the run continues with logging simply absent.
	Info(t.Context(), "the command keeps working", slog.String("path", "harnaas.json"))
}

func TestCloseIsSafeToCallRepeatedly(t *testing.T) {
	silenceDefault(t)
	logTo(t)

	Info(t.Context(), "recorded")
	Close()
	Close()

	Info(t.Context(), "discarded")
}
