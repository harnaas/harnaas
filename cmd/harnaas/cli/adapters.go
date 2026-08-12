package cli

// The named harness adapters this binary ships.
//
// Each adapter package registers itself with adapter.Default from its own init,
// so what decides whether harnaas can install a rule, a command or a persona for
// a harness is whether the binary links its package — and this file is where
// that decision is made and reviewed. The import is blank because nothing here
// calls into an adapter: the install flow asks adapter.Default for the harness
// it is addressing, which is the whole point of dispatching through a registry.
//
// Most harnesses need no entry here at all. A skill and an instruction reach
// them through shared locations, so an adapter is what a harness's own rule,
// command and persona surfaces need — and "a harness with no adapter" is a
// supported state rather than a gap in this list.
//
// TestBinaryRegistersTheDeclaredAdapters pins the current set, so neither adding
// one nor losing one happens by accident.
import (
	// Blank: linked for the side effect of registering the adapter.
	_ "github.com/harnaas/harnaas/cmd/harnaas/cli/adapter/claudecode"
	// Blank: linked for the side effect of registering the adapter.
	_ "github.com/harnaas/harnaas/cmd/harnaas/cli/adapter/devincli"
)
