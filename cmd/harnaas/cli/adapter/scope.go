package adapter

import (
	"fmt"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// ResolveRoot returns the harness directory an asset installs beneath: the
// adapter's own root for the asset's scope, relative to that scope's own root —
// the project root at `project` scope, the user's home at `user` scope.
//
// Which scope an asset has is settled long before this: [manifest.Interpret]
// defaults an undeclared one to `project`, refuses a scope harnaas does not
// define, and refuses `user` where the *roster* records no per-user location.
// What is left for this layer is the question only an adapter can answer — does
// this harness have an unambiguous location for that scope — and the roster's
// answer is deliberately not taken as this one. The roster is a fact about a
// harness the manifest layer can check without linking a single adapter; the
// adapter is the thing that actually knows its own directories, and where the
// two disagree the adapter is right.
//
// A refusal is a refusal, never a quieter install: an asset that asked for
// `user` scope and was given the project root would be installed somewhere its
// author did not ask for, in a file they would have no reason to look at.
func ResolveRoot(a Adapter, asset manifest.Asset) (string, error) {
	scope := asset.Scope
	if scope == "" {
		// An Asset always carries a scope, because Interpret is the only route
		// to one and it defaults an undeclared field. The restatement is not a
		// second place the default is decided — it is what stops a zero value
		// reaching a.Root("") and producing a diagnostic about a scope nobody
		// wrote.
		scope = manifest.ScopeProject
	}

	root, offered := a.Root(scope)
	if !offered {
		return "", &UnsupportedScopeError{
			AssetID: asset.ID,
			Type:    asset.Type,
			Harness: a.Harness(),
			Scope:   scope,
		}
	}

	return root, nil
}

// UnsupportedScopeError reports an asset whose scope this harness has no
// unambiguous directory for.
//
// It names the asset and the harness because the pairing is what is wrong:
// `user` scope is correct for the asset and the harness is correct for the
// project, and only together do they name a location harnaas would have to
// invent. Inventing one is the outcome this refusal exists to prevent — an
// asset installed under a guessed per-user directory reports success, is never
// read, and leaves the team believing it is in effect.
type UnsupportedScopeError struct {
	// AssetID is the asset that asked for the scope.
	AssetID string

	// Type is the asset's type, which is how the message names it in the
	// author's own vocabulary rather than as "the asset".
	Type manifest.AssetType

	// Harness is the harness with no directory for that scope.
	Harness harness.ID

	// Scope is the scope that could not be resolved.
	Scope manifest.Scope
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *UnsupportedScopeError) Error() string {
	return fmt.Sprintf(
		"the %s %q installs at %q scope, and the harness %q has no unambiguous location for that scope\n\n"+
			"%s",
		e.Type, e.AssetID, e.Scope, e.Harness,
		e.remedy(),
	)
}

// remedy names the edit that fixes it, which depends on which scope was asked
// for. Dropping the scope is an edit only where there is a scope to drop: at
// `project` scope there is nothing to remove and the entry's only route to a
// harness that can take it is its `targets`.
func (e *UnsupportedScopeError) remedy() string {
	if e.Scope == manifest.ScopeUser {
		return fmt.Sprintf(
			"Remove %q from the entry in %s so it installs at %q scope, or narrow the entry's %q to harnesses with a per-user location.",
			"scope", manifest.FileName, manifest.ScopeProject, "targets",
		)
	}

	return fmt.Sprintf(
		"Remove %q from the entry's %q in %s.",
		e.Harness, "targets", manifest.FileName,
	)
}
