package source

import (
	"os"
	"testing"

	"github.com/harnaas/harnaas/internal/testenv"
)

// This package resolves the archive cache's default location from the user's
// cache directory, so its tests run with the per-user directories redirected
// under a temporary one. Without it, a test that forgot to set
// HARNAAS_CACHE_DIR would fill the cache of whoever ran the suite — and pass.
func TestMain(m *testing.M) {
	os.Exit(testenv.Main(m))
}
