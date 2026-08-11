package logging

import (
	"os"
	"testing"

	"github.com/harnaas/harnaas/internal/testenv"
)

// This package resolves its default log location from the user's cache
// directory, so its tests run with the per-user directories redirected under a
// temporary one. Without it, a test that forgot to set HARNAAS_LOG_FILE would
// append to the log of whoever ran the suite — and pass.
func TestMain(m *testing.M) {
	os.Exit(testenv.Main(m))
}
