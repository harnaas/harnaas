package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// scaffoldDirPerm and scaffoldFilePerm are the modes the local asset scaffolding
// is created with: the ordinary non-executable defaults, because what lands here
// is a directory a person opens and a document they read.
const (
	scaffoldDirPerm  fs.FileMode = 0o755
	scaffoldFilePerm fs.FileMode = 0o644
)

// scaffoldExplanation is the file each created directory carries.
//
// It exists for two reasons at once, and the second is the load-bearing one. It
// answers "what goes in here" where the question is asked, and it makes the
// directory something git will track — an empty directory is never committed, so
// scaffolding without a file in it would exist only for the person who ran init,
// who is the one person who did not need it.
const scaffoldExplanation = "README.md"

// scaffoldDir is one directory the scaffolding creates, and the asset type it is
// for. The root itself has no type, which is why it is not one of these.
type scaffoldDir struct {
	// Type is the asset type this directory holds.
	Type manifest.AssetType

	// Path is project-root-relative and slash-separated.
	Path string
}

// scaffoldResult is what the scaffolding did, in the terms the report needs.
//
// Created and Existing are separate because claiming to have created a directory
// that was already there is the one thing a report of this kind must not do: the
// author may have put something in it, and a run that says it made it is a run
// they will not look at again.
type scaffoldResult struct {
	// Created holds the project-root-relative paths this run created, in the
	// order they were created.
	Created []string

	// Existing holds the ones that were already there.
	Existing []string
}

// scaffoldLocalAssets creates the project's local asset directory and the
// asset-type directories the selection can receive.
//
// Everything here only ever adds. A directory that exists is left alone, an
// explanation that exists is never overwritten, and nothing is removed — on any
// flag, including a forced run, and including a run whose selection is narrower
// than an earlier one's. `.harnaas` holds content the author wrote and harnaas
// only ever reads: it is never a destination, so nothing here can be part of the
// managed set the lockfile records, and nothing here is harnaas's to replace.
// See ADR 0006.
//
// The writes go through a handle anchored at the project root. The paths are
// constants, so containment is not in doubt textually — but `.harnaas` is a live
// directory that may be a symbolic link to somewhere else on this machine, and
// creating through the anchor is what makes that the kernel's answer rather than
// harnaas's assumption. It is the rule the local source kind already applies when
// reading the same directory.
func scaffoldLocalAssets(root string, targets []harness.ID, registry *adapter.Registry) (scaffoldResult, error) {
	anchor, err := os.OpenRoot(root)
	if err != nil {
		return scaffoldResult{}, fmt.Errorf("open project root %s: %w", root, err)
	}
	defer func() { _ = anchor.Close() }()

	var result scaffoldResult

	// The root carries no explanation of its own: what belongs in it is the
	// directories below, each of which says so for itself.
	created, err := createDirectory(anchor, manifest.LocalRoot)
	if err != nil {
		return result, err
	}
	result.record(manifest.LocalRoot, created)

	for _, directory := range scaffoldDirectories(targets, registry) {
		created, err := createDirectory(anchor, directory.Path)
		if err != nil {
			return result, err
		}
		result.record(directory.Path, created)

		if !created {
			// Somebody's directory, explanation included.
			continue
		}
		if err := writeExplanation(anchor, directory); err != nil {
			return result, err
		}
	}

	return result, nil
}

// record files one directory under what happened to it.
func (r *scaffoldResult) record(directory string, created bool) {
	if created {
		r.Created = append(r.Created, directory)
		return
	}
	r.Existing = append(r.Existing, directory)
}

// scaffoldDirectories is the set of asset-type directories a selection earns, in
// the manifest's own type order.
//
// Every type is asked the same question — could an asset of this type, declaring
// nothing beyond its path, reach at least one selected harness — and the question
// is answered by the routing an install uses. A directory offered for a pairing an
// install would refuse invites an author to write something harnaas has already
// decided it cannot deliver; one withheld from a pairing an install would accept
// hides a type the project can use.
//
// That a skill and an instruction come back for every selection is a consequence
// of every recognized harness reaching both through shared locations, not an
// exception written in here.
func scaffoldDirectories(targets []harness.ID, registry *adapter.Registry) []scaffoldDir {
	var directories []scaffoldDir
	for _, assetType := range manifest.AssetTypes() {
		if !reachesAnyTarget(assetType, targets, registry) {
			continue
		}
		directory, known := manifest.DirectoryFor(assetType)
		if !known {
			// Unreachable: the types walked here are the manifest's own, and
			// the lookup is the inference table read backwards. Skipping is the
			// only sane answer to a type with nowhere to live.
			continue
		}
		directories = append(directories, scaffoldDir{
			Type: assetType,
			Path: path.Join(manifest.LocalRoot, directory),
		})
	}
	return directories
}

// reachesAnyTarget reports whether any selected harness could receive this type.
func reachesAnyTarget(assetType manifest.AssetType, targets []harness.ID, registry *adapter.Registry) bool {
	for _, target := range targets {
		if typeReachesHarness(target, assetType, registry) {
			return true
		}
	}
	return false
}

// createDirectory creates one directory through the anchor, reporting whether
// this run is what created it.
//
// An existing directory is not an error and not a conflict: the author put it
// there, or an earlier run did, and either way there is nothing to do and nothing
// to say beyond not claiming it.
func createDirectory(anchor *os.Root, directory string) (bool, error) {
	if err := anchor.Mkdir(filepath.FromSlash(directory), scaffoldDirPerm); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, &scaffoldError{Path: directory, cause: err}
	}
	return true, nil
}

// writeExplanation writes a directory's README, and never over one.
//
// The write is create-if-absent rather than the staged-and-renamed atomic write
// the rest of harnaas uses, because a rename replaces whatever is there and this
// file is the author's from the moment it exists. Never touching their version
// outranks the atomicity of a file nothing parses: a half-written explanation on
// an interrupted run is a cosmetic loss, and a rewritten one is harnaas editing
// content it does not own.
func writeExplanation(anchor *os.Root, directory scaffoldDir) error {
	explanation := path.Join(directory.Path, scaffoldExplanation)

	file, err := anchor.OpenFile(
		filepath.FromSlash(explanation),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		scaffoldFilePerm,
	)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return &scaffoldError{Path: explanation, cause: err}
	}
	defer func() { _ = file.Close() }()

	if _, err := file.WriteString(explanationFor(directory)); err != nil {
		return &scaffoldError{Path: explanation, cause: err}
	}
	if err := file.Sync(); err != nil {
		return &scaffoldError{Path: explanation, cause: err}
	}
	return nil
}

// scaffoldError reports a directory or explanation harnaas could not create.
//
// It says the manifest was created, because by the time anything here runs it
// was: the project is initialized, the remaining work is the scaffolding, and a
// re-run completes it. A message that only named the failure would leave its
// reader unsure whether to start over.
type scaffoldError struct {
	// Path is what could not be created, relative to the project root.
	Path string

	cause error
}

func (e *scaffoldError) Error() string {
	return fmt.Sprintf(
		"created %s, but could not create %s: %v\n\n"+
			"The project is initialized. Clear what is in the way and re-run "+
			"`harnaas init --force` to finish creating %s, or create the directories yourself.",
		manifest.FileName, e.Path, e.cause, manifest.LocalRoot,
	)
}

func (e *scaffoldError) Unwrap() error { return e.cause }
