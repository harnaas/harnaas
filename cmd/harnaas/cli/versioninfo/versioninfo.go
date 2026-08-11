// Package versioninfo reports which harnaas build is running.
//
// Version and Commit are stamped by goreleaser via ldflags on a release build.
// Every other build — `mise run build`, `go install
// github.com/harnaas/harnaas/cmd/harnaas@<version>`, a plain `go build` —
// carries no stamp, so Load recovers what it can from Go's embedded build
// information and the binary still self-reports something truthful.
package versioninfo

import (
	"runtime/debug"
	"strings"
)

// The placeholders Load treats as "unstamped". A build that carries a real
// stamp keeps it: an explicit stamp always beats the embedded build info.
const (
	devVersion    = "dev"
	unknownCommit = "unknown"
)

// Version and Commit identify the running harnaas build. goreleaser overwrites
// both through -X ldflags; see .goreleaser.yaml.
var (
	Version = devVersion
	Commit  = unknownCommit
)

// Load fills Version and Commit from the binary's embedded build information
// where ldflags left them at their defaults. Call it once from main, before
// anything reads either variable.
func Load() {
	info, ok := debug.ReadBuildInfo()
	Version, Commit = resolve(Version, Commit, info, ok)
}

// resolve is Load's pure core, so the precedence rules are testable without a
// real build. A module install (`@<version>`) carries the version as
// info.Main.Version; a local build reports "(devel)" there but records the
// commit under vcs.revision. Go already marks a dirty tree with a "+dirty"
// suffix on the version, so there is no need to read vcs.modified.
func resolve(version, commit string, info *debug.BuildInfo, ok bool) (string, string) {
	if version != devVersion || !ok || info == nil {
		return version, commit
	}

	if v := info.Main.Version; v != "" && v != "(devel)" {
		version = strings.TrimPrefix(v, "v") // match goreleaser's {{.Version}}
	}
	if commit == unknownCommit {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				commit = setting.Value
			}
		}
	}

	return version, commit
}
