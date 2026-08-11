package source

import (
	"fmt"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Every diagnostic here is about content that arrived intact and is not what the
// asset said it was, so none of them names a host, a commit or a credential.
// What each one names is the asset, because the edit that fixes it is in
// `harnaas.json` or in the source the entry points at, and a reader who has both
// open needs to know which entry to look at first.

// ShapeError reports content whose shape is not the one its asset type requires.
//
// The expected and the found shape are both carried as text rather than derived
// from the type at render time, because the two halves of the sentence come from
// two different places — the type's rule, and the directory somebody is looking
// at — and a message that restated the rule twice would leave the reader to work
// out which half was the observation.
type ShapeError struct {
	// AssetID is the asset whose source was verified.
	AssetID string

	// Type is the asset's type, which is where the requirement comes from.
	Type manifest.AssetType

	// Expected is the shape the type requires, in the words the message uses.
	Expected string

	// Found is what the source resolved to instead.
	Found string
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *ShapeError) Error() string {
	return fmt.Sprintf(
		"the asset %q is a %s, which must be %s, and its source is %s\n\n"+
			"Point the entry at %s, or change the entry's %q in %s to match what the source is.",
		e.AssetID, e.Type, e.Expected, e.Found,
		e.Expected, "type", manifest.FileName,
	)
}

// MissingSkillFileError reports a skill directory with no SKILL.md in it.
//
// It is separate from [ShapeError] because the source is the right shape and the
// edit is in the content rather than in the manifest: adding a file, or pointing
// the entry one directory deeper. Reporting it as a shape mismatch would send an
// author to change a `type` that is correct.
type MissingSkillFileError struct {
	// AssetID is the asset whose source was verified.
	AssetID string

	// Source is the path the entry declared, as the manifest spells it.
	Source string
}

// Error states the problem and then the fix.
func (e *MissingSkillFileError) Error() string {
	return fmt.Sprintf(
		"the skill %q reads from %s, which holds no %s\n\n"+
			"Add %s to %s, or point the entry at the directory that has one.",
		e.AssetID, e.Source, SkillFileName,
		SkillFileName, e.Source,
	)
}

// SkillFrontmatterError reports a SKILL.md harnaas could not read a name out of.
//
// Absent, unparseable and present-without-a-name are one diagnostic with three
// reasons rather than three types, because the reader's next action is the same
// in all three — open the file and look at the top of it — and the reason is
// what tells them what they will find when they do.
type SkillFrontmatterError struct {
	// AssetID is the asset whose source was verified.
	AssetID string

	// Reason is what is wrong with the block, in the words the message uses.
	Reason string

	// Err is the parse failure where there was one, kept so the position the
	// YAML parser reported stays inspectable. It is nil for the two reasons that
	// are not parse failures.
	Err error
}

// Error states the problem and then the fix.
func (e *SkillFrontmatterError) Error() string {
	return fmt.Sprintf(
		"the skill %q has a %s harnaas cannot read a name from: %s\n\n"+
			"Give %s a frontmatter block whose %q is %q, between two %q lines at the top of the file.",
		e.AssetID, SkillFileName, e.Reason,
		SkillFileName, "name", e.AssetID, "---",
	)
}

// Unwrap keeps the parse failure inspectable where there was one.
func (e *SkillFrontmatterError) Unwrap() error { return e.Err }

// SkillNameMismatchError reports a skill whose frontmatter names something other
// than the asset it is installed as.
//
// This is the failure the whole check exists for. A harness that reads the
// `name` field uses it to decide the skill exists at all, so a mismatch installs
// cleanly, reports success and is never invoked — which is the one outcome a
// tool whose purpose is telling a team what is in effect must not produce.
// harnaas refuses rather than rewriting the field, because `SKILL.md` is the
// author's file and a frontmatter harnaas re-serialized is not the one they
// reviewed.
type SkillNameMismatchError struct {
	// AssetID is the id harnaas installs the skill as, declared by the entry or
	// inferred from its path.
	AssetID string

	// DeclaredName is the name the skill's own frontmatter gives.
	DeclaredName string
}

// Error states the problem and then the fix.
func (e *SkillNameMismatchError) Error() string {
	return fmt.Sprintf(
		"the skill installs as %q and its %s declares the name %q, and a harness reading that field would not find it\n\n"+
			"Set the frontmatter %q to %q, or give the entry an %q of %q in %s.",
		e.AssetID, SkillFileName, e.DeclaredName,
		"name", e.AssetID, "id", e.DeclaredName, manifest.FileName,
	)
}
