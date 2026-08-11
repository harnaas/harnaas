package adapter

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Registry is the set of named adapters this binary can install through, keyed
// by the harness each one addresses.
//
// A registry value rather than one package-level map is what lets a test
// exercise lookup — including the failure for a harness nobody wrote an adapter
// for — without reaching into the set the binary itself ships.
type Registry struct {
	mu sync.Mutex

	// adapters holds each adapter as a value rather than a constructor, which is
	// deliberately unlike the source-kind registry: a kind remembers which
	// archives this run has already fetched and so belongs to the run, while an
	// adapter is a mapping from an asset to a path and has nothing to remember.
	// Handing every run its own copy of a pure mapping would suggest it did.
	adapters map[harness.ID]Adapter
}

// Default is the registry the adapters shipped with this binary register into,
// and the one the install flow resolves destinations through. An adapter package
// registers itself from its own init, so an adapter that has been linked in is an
// adapter that works.
var Default = &Registry{}

// Register adds an adapter to [Default].
func Register(adapter Adapter) {
	Default.Register(adapter)
}

// Register adds an adapter under the harness it addresses.
//
// A second adapter for one harness panics rather than replacing or ignoring the
// first. Registration happens during package initialization, before the process
// has read anything, so the only way to reach it is a mistake in harnaas's own
// wiring — and both silent outcomes are a binary that installs through whichever
// adapter happened to be linked last.
func (r *Registry) Register(adapter Adapter) {
	id := adapter.Harness()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.adapters == nil {
		r.adapters = make(map[harness.ID]Adapter)
	}
	if _, registered := r.adapters[id]; registered {
		panic(fmt.Sprintf("adapter: harness %q has two adapters registered", id))
	}
	r.adapters[id] = adapter
}

// Lookup returns the adapter for a harness.
//
// A harness with no adapter is not a broken install by itself: skills and
// instructions reach every harness through shared locations, and this lookup only
// happens for the types that have no shared equivalent. That is why the failure
// is a typed diagnostic the caller can attribute to the asset that needed it,
// rather than something the registry decides is fatal.
func (r *Registry) Lookup(id harness.ID) (Adapter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	adapter, registered := r.adapters[id]
	if !registered {
		return nil, &NotRegisteredError{Harness: id, Registered: r.harnessesLocked()}
	}
	return adapter, nil
}

// Registered reports whether a harness has a named adapter.
//
// It is separate from [Registry.Lookup] because the install flow asks the
// question long before it has an asset to blame for the answer: a harness with no
// adapter still installs every shared asset, and building a diagnostic to throw
// away would be the wrong shape for that path.
func (r *Registry) Registered(id harness.ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, registered := r.adapters[id]
	return registered
}

// Harnesses returns every harness with a named adapter, sorted.
//
// Sorted rather than in registration order, because registration order is
// whatever the import graph happens to produce and this list is rendered into the
// message a user reads when their harness is not among them.
func (r *Registry) Harnesses() []harness.ID {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.harnessesLocked()
}

// Adapters returns every registered adapter, in [Registry.Harnesses] order, so a
// caller that walks them reports the same thing on every run.
func (r *Registry) Adapters() []Adapter {
	r.mu.Lock()
	defer r.mu.Unlock()

	adapters := make([]Adapter, 0, len(r.adapters))
	for _, id := range r.harnessesLocked() {
		adapters = append(adapters, r.adapters[id])
	}
	return adapters
}

// harnessesLocked returns the registered harnesses in sorted order. The caller
// holds the lock.
func (r *Registry) harnessesLocked() []harness.ID {
	ids := make([]harness.ID, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// NotRegisteredError reports a harness this binary has no named adapter for.
//
// The manifest layer has already accepted the name, because the roster
// recognizes more harnesses than there are adapters and that is the intended
// state: a harness harnaas knows about receives every shared asset without one.
// So the problem is never "this harness does not exist" — it is that the asset in
// hand is of a type with no shared equivalent, and the message has to say which
// harnesses could have taken it.
type NotRegisteredError struct {
	// Harness is the harness with no adapter, exactly as the manifest wrote it.
	Harness harness.ID

	// Registered is every harness this binary has an adapter for, sorted.
	Registered []harness.ID
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *NotRegisteredError) Error() string {
	return fmt.Sprintf(
		"harnaas has no adapter for the harness %q, and a %s, %s or %s asset has no shared location to install to\n\n"+
			"%s Skills and instructions need no adapter and install for every harness harnaas recognizes.",
		e.Harness,
		manifest.AssetTypeRule, manifest.AssetTypeCommand, manifest.AssetTypePersona,
		e.remedy(),
	)
}

// remedy names the harnesses that would have worked, or says plainly that there
// are none. An empty registry is unreachable from a built binary — it links at
// least one adapter — but a message reading "target one of: ." is worse than one
// stating the situation.
func (e *NotRegisteredError) remedy() string {
	if len(e.Registered) == 0 {
		return fmt.Sprintf("This harnaas has no adapters at all, so remove the entry from %s.", manifest.FileName)
	}

	names := make([]string, 0, len(e.Registered))
	for _, id := range e.Registered {
		names = append(names, fmt.Sprintf("%q", id))
	}

	return fmt.Sprintf(
		"Change the entry's %q in %s to a harness harnaas has an adapter for: %s.",
		"targets", manifest.FileName, strings.Join(names, ", "),
	)
}
