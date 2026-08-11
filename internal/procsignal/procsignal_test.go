package procsignal

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests mutate the package-global recorded signal, so they must not run
// in parallel with each other.

func TestLoadReportsTheStoredSignal(t *testing.T) {
	Reset()
	require.Nil(t, Load(), "no signal recorded yet")

	Store(os.Interrupt)
	require.Equal(t, os.Interrupt, Load())

	// A different concrete signal type must not panic the atomic store, and
	// must win: the last signal observed is the one that terminates us.
	Store(syscall.SIGTERM)
	require.Equal(t, os.Signal(syscall.SIGTERM), Load())
}

func TestResetClearsTheStoredSignal(t *testing.T) {
	Store(os.Interrupt)
	Reset()
	require.Nil(t, Load())
}
