package cli

// The source kinds this binary ships.
//
// Each kind package registers itself with source.Default from its own init, so
// what decides whether harnaas can resolve a kind is whether the binary links
// its package — and this file is where that decision is made and reviewed. The
// imports are blank because nothing here calls into a kind: the install flow
// asks source.Default for a resolver and hands it a request, which is the whole
// point of dispatching through a registry.
//
// Adding a forge is therefore one line here plus a package, and removing one is
// one line here. TestBinaryRegistersExactlyTheV1SourceKinds pins the current set
// so neither happens by accident.
import (
	// Blank: linked for the side effect of registering the kind.
	_ "github.com/harnaas/harnaas/cmd/harnaas/cli/source/github"
	// Blank: linked for the side effect of registering the kind.
	_ "github.com/harnaas/harnaas/cmd/harnaas/cli/source/local"
)
