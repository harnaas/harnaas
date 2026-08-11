package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/logging"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
)

// The logging guardrails, checked where the commands are rather than inside the
// logging package: that a run's diagnostics reach the log file and neither of
// the command's streams, and that what reaches the file is the path to a file
// and never what was in it.
//
// Neither test can use t.Parallel, because both point HARNAAS_LOG_FILE at a
// scratch file with t.Setenv and the package logger is process-wide.

// logToScratchFile directs harnaas's log at a file under t.TempDir for the
// duration of one test, and returns the records written to it. No test reads or
// writes the real per-user log.
func logToScratchFile(t *testing.T) func() []map[string]any {
	t.Helper()

	path := filepath.Join(t.TempDir(), "harnaas.log")
	t.Setenv(logging.FileEnvVar, path)

	closeLog := logging.Open()
	t.Cleanup(closeLog)

	return func() []map[string]any {
		// Closing first flushes and releases the file, which Windows requires
		// before it can be read.
		closeLog()

		raw, err := os.ReadFile(path)
		require.NoError(t, err, "harnaas wrote no log file at all")

		var records []map[string]any
		for line := range strings.Lines(strings.TrimSpace(string(raw))) {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &record), "log line is not JSON: %s", line)
			records = append(records, record)
		}
		return records
	}
}

// TestARunsDiagnosticsReachTheLogFileAndNeitherStream is the whole reason
// harnaas logs to a file: a diagnostic on stdout would corrupt a --json
// document mid-parse, and one on stderr would interleave with the text the user
// is reading. `init` is the command that exists to check it with.
func TestARunsDiagnosticsReachTheLogFileAndNeitherStream(t *testing.T) {
	logged := logToScratchFile(t)

	run := runInitIn(t, scratchProject(t))
	require.NoError(t, run.err)

	records := logged()
	require.NotEmpty(t, records, "the run recorded nothing")

	created := findRecord(t, records, "manifest created")
	assert.Equal(t, run.manifestPath(), created["path"])
	assert.InDelta(t, 1.0, created["harness_count"], 0)

	for name, stream := range map[string]string{"stdout": run.stdout, "stderr": run.stderr} {
		assert.NotContains(t, stream, "manifest created", "a log record reached %s", name)
		assert.NotContains(t, stream, `"level"`, "a log record reached %s", name)
	}
}

// TestALogRecordNamesAFileAndNeverItsContents is the privacy rule as a test.
//
// The files harnaas handles are a team's instructions and rules, which is
// exactly the content nobody expects to find copied into a log they did not
// know existed. So a record may say which file harnaas read and what happened;
// it may not say what was in it.
//
// The sentinel is planted as a field name in the manifest, so the decode
// failure quotes it back — the assertion below that the error message contains
// it is the point of the test, not incidental. That message is a good
// diagnostic and a leak: logging err.Error() would be the natural thing to
// write, and would put the author's own file in the log. What a command records
// instead is the path and the failure's type, which is the shape both call
// sites in this binary already use.
func TestALogRecordNamesAFileAndNeverItsContents(t *testing.T) {
	logged := logToScratchFile(t)

	const sentinel = "the_authors_own_words"

	root := scratchProject(t)
	writeFile(t, root, manifest.FileName, fmt.Sprintf(`{
  "version": 1,
  "harnesses": ["claude-code"],
  %q: true
}
`, sentinel))

	ctx := paths.WithProjectRoot(t.Context(), root)
	path, err := manifest.Path(ctx)
	require.NoError(t, err)

	doc, err := manifest.Load(ctx)
	require.Error(t, err)
	require.Nil(t, doc)
	require.Contains(t, err.Error(), sentinel,
		"the diagnostic quotes the author's own file, which is why the message must never be what is logged")

	logging.Error(ctx, "manifest failed to load",
		slog.String("path", path),
		slog.String("error_type", fmt.Sprintf("%T", err)),
	)

	records := logged()
	failed := findRecord(t, records, "manifest failed to load")
	assert.Equal(t, path, failed["path"], "the path is an identifier and belongs in the log")
	assert.Equal(t, "*manifest.DecodeError", failed["error_type"])

	for _, record := range records {
		for key, value := range record {
			assert.NotContains(t, fmt.Sprint(value), sentinel,
				"the file's own content reached the log under key %q", key)
		}
	}
}

// findRecord returns the one record with the given message.
func findRecord(t *testing.T, records []map[string]any, msg string) map[string]any {
	t.Helper()

	for _, record := range records {
		if record["msg"] == msg {
			return record
		}
	}

	require.FailNowf(t, "no such log record", "no record with msg %q in %v", msg, records)
	return nil
}
