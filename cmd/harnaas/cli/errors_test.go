package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSentinel = errors.New("manifest is missing")

func TestAlreadyPrintedErrorKeepsItsCauseReachable(t *testing.T) {
	t.Parallel()

	err := NewAlreadyPrintedError(fmt.Errorf("load manifest: %w", errSentinel))

	assert.Equal(t, "load manifest: manifest is missing", err.Error())
	require.ErrorIs(t, err, errSentinel, "marking an error as printed must not hide its cause")

	var printed *AlreadyPrintedError
	require.ErrorAs(t, error(err), &printed)
	assert.True(t, printed.AlreadyPrinted())
}

// TestAlreadyPrintedErrorSurvivesFurtherWrapping guards the entrypoint's
// contract: it detects the mark with errors.As, so a caller that adds context
// on the way out must not defeat the detection.
func TestAlreadyPrintedErrorSurvivesFurtherWrapping(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("run init: %w", NewAlreadyPrintedError(errSentinel))

	var printed *AlreadyPrintedError
	require.ErrorAs(t, wrapped, &printed)
	require.ErrorIs(t, wrapped, errSentinel)
}
