package versioninfo

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func buildInfo(mainVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main:     debug.Module{Path: "github.com/harnaas/harnaas", Version: mainVersion},
		Settings: settings,
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		version     string
		commit      string
		info        *debug.BuildInfo
		ok          bool
		wantVersion string
		wantCommit  string
	}{
		{
			name:        "an explicit stamp beats the embedded build info",
			version:     "0.1.0",
			commit:      "abc1234",
			info:        buildInfo("v9.9.9", debug.BuildSetting{Key: "vcs.revision", Value: "deadbeef"}),
			ok:          true,
			wantVersion: "0.1.0",
			wantCommit:  "abc1234",
		},
		{
			name:        "a module install recovers the tagged version",
			version:     devVersion,
			commit:      unknownCommit,
			info:        buildInfo("v0.1.0"),
			ok:          true,
			wantVersion: "0.1.0",
			wantCommit:  unknownCommit,
		},
		{
			name:    "a local build recovers the commit from vcs.revision",
			version: devVersion,
			commit:  unknownCommit,
			info: buildInfo("(devel)",
				debug.BuildSetting{Key: "vcs.revision", Value: "ddf1a331c0ffee1234567890abcdef0987654321"},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"},
			),
			ok:          true,
			wantVersion: devVersion,
			wantCommit:  "ddf1a331c0ffee1234567890abcdef0987654321",
		},
		{
			name:        "an explicit commit survives the build-info fallback",
			version:     devVersion,
			commit:      "abc1234",
			info:        buildInfo("(devel)", debug.BuildSetting{Key: "vcs.revision", Value: "deadbeef"}),
			ok:          true,
			wantVersion: devVersion,
			wantCommit:  "abc1234",
		},
		{
			name:        "no build info leaves the development placeholders",
			version:     devVersion,
			commit:      unknownCommit,
			info:        nil,
			ok:          false,
			wantVersion: devVersion,
			wantCommit:  unknownCommit,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotVersion, gotCommit := resolve(tc.version, tc.commit, tc.info, tc.ok)
			assert.Equal(t, tc.wantVersion, gotVersion, "version")
			assert.Equal(t, tc.wantCommit, gotCommit, "commit")
		})
	}
}

// TestLoadIsSafeToCallOnAnUnstampedBinary covers the path main actually takes:
// the test binary carries no ldflags stamp, so Load must leave the variables
// populated rather than blank.
func TestLoadIsSafeToCallOnAnUnstampedBinary(t *testing.T) {
	Load()

	assert.NotEmpty(t, Version)
	assert.NotEmpty(t, Commit)
}
