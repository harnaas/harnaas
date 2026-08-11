package cli

import (
	"os"
	"testing"

	"github.com/harnaas/harnaas/internal/testenv"
)

// The commands in this package open the log file and write into a project, so
// the suite runs with the per-user directories redirected under a temporary
// one. A test that reached a per-user location without meaning to would
// otherwise leave the developer's own machine changed, and pass.
func TestMain(m *testing.M) {
	os.Exit(testenv.Main(m))
}
