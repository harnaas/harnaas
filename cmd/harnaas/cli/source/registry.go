package source

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Registry is the set of source kinds harnaas can resolve, keyed by the name a
// manifest writes.
//
// It exists so that adding a kind is additive: the install flow asks the
// registry for a resolver and hands it a request, and contains no branch on
// which kind it got. A registry value rather than one package-level map is what
// lets a test exercise dispatch — including the failure for a kind nobody
// registered — without reaching into the set the binary itself ships.
type Registry struct {
	mu sync.Mutex

	// kinds holds each kind's constructor rather than a kind, because a kind is
	// per run. See [Kind].
	kinds map[manifest.SourceKind]NewKind
}

// Default is the registry the kinds shipped with this binary register into, and
// the one the install flow resolves through. A kind package registers itself
// from its own init, so a kind that has been linked in is a kind that works.
var Default = &Registry{}

// Register adds a kind to [Default].
func Register(name manifest.SourceKind, newKind NewKind) {
	Default.Register(name, newKind)
}

// Register adds a kind under the name a manifest writes for it.
//
// A second registration of the same name panics rather than replacing or
// ignoring the first. Registration happens during package initialization, before
// the process has read anything, so the only way to reach it is a mistake in
// harnaas's own wiring — and the two silent outcomes are a binary that resolves
// through whichever kind happened to be linked last.
func (r *Registry) Register(name manifest.SourceKind, newKind NewKind) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.kinds == nil {
		r.kinds = make(map[manifest.SourceKind]NewKind)
	}
	if _, registered := r.kinds[name]; registered {
		panic(fmt.Sprintf("source: kind %q is registered twice", name))
	}
	r.kinds[name] = newKind
}

// Kinds returns every registered kind name, sorted.
//
// Sorted rather than in registration order, because registration order is
// whatever the import graph happens to produce and this list is rendered into
// the error a user reads when their kind is not among them.
func (r *Registry) Kinds() []manifest.SourceKind {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.kindNamesLocked()
}

// NewResolver constructs one resolver per registered kind and returns the
// [Resolver] that dispatches to them, which is the unit of one install run.
func (r *Registry) NewResolver() *Resolver {
	r.mu.Lock()
	defer r.mu.Unlock()

	kinds := make(map[manifest.SourceKind]Kind, len(r.kinds))
	for name, newKind := range r.kinds {
		kinds[name] = newKind()
	}
	return &Resolver{kinds: kinds, registered: r.kindNamesLocked()}
}

// kindNamesLocked returns the registered names in sorted order. The caller holds
// the lock.
func (r *Registry) kindNamesLocked() []manifest.SourceKind {
	names := make([]manifest.SourceKind, 0, len(r.kinds))
	for name := range r.kinds {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Resolver resolves every source of one install run.
//
// It holds one live kind per registered name, so the per-run state a kind keeps
// — chiefly which repository archives it has already fetched — lives exactly as
// long as the run that built it.
type Resolver struct {
	kinds map[manifest.SourceKind]Kind

	// registered is the sorted name list, captured at construction so the error
	// for an unsupported kind lists what this run could have resolved rather
	// than what the registry holds by the time somebody reads the message.
	registered []manifest.SourceKind
}

// Resolve resolves one asset's source through the kind that owns it.
//
// The lookup happens before the kind is asked to do anything, which is what
// makes "an unsupported kind reaches no network and writes no file" a property
// of the dispatch rather than a rule each kind has to honour on its way in.
func (r *Resolver) Resolve(ctx context.Context, req Request) (*Resolved, error) {
	name := req.Kind()

	kind, registered := r.kinds[name]
	if !registered {
		return nil, &UnsupportedKindError{Kind: name, Registered: slices.Clone(r.registered)}
	}

	return kind.Resolve(ctx, req)
}

// UnsupportedKindError reports a source kind no registered kind can resolve.
//
// The manifest layer rejects a kind its grammar does not know, so reaching this
// means a kind harnaas parses but this binary cannot fetch — an older binary
// meeting a manifest written for a newer one. That is why the remedy is an
// upgrade rather than an edit, and why the error is a type: the install flow
// attributes it to the asset that carried the source.
type UnsupportedKindError struct {
	// Kind is the unresolvable kind, exactly as the manifest wrote it.
	Kind manifest.SourceKind

	// Registered is every kind this binary can resolve, sorted.
	Registered []manifest.SourceKind
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *UnsupportedKindError) Error() string {
	names := make([]string, 0, len(e.Registered))
	for _, name := range e.Registered {
		names = append(names, string(name))
	}

	return fmt.Sprintf(
		"this harnaas cannot resolve sources of kind %q\n\n"+
			"Upgrade harnaas, or declare a source of a kind it installs: %s.",
		e.Kind, strings.Join(names, ", "),
	)
}
