package manifest

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// AssetType is what an asset is, in the vocabulary the harnesses and this
// project share: a skill, a rule, an instruction, a command or a persona.
type AssetType string

const (
	// AssetTypeSkill is an asset the harness loads on its own initiative when
	// its description matches the task at hand.
	AssetTypeSkill AssetType = "skill"

	// AssetTypeRule is always-on guidance installed as its own file.
	AssetTypeRule AssetType = "rule"

	// AssetTypeInstruction is always-on guidance concatenated into a managed
	// block in the project's committed memory file.
	AssetTypeInstruction AssetType = "instruction"

	// AssetTypeCommand is an asset the user invokes deliberately.
	AssetTypeCommand AssetType = "command"

	// AssetTypePersona is a delegated worker with its own model and tool budget.
	AssetTypePersona AssetType = "persona"
)

// assetTypeDirectories maps the directory that contains an asset to the type it
// names.
//
// The convention is not invented here: `skills/`, `commands/` and `agents/` are
// already how asset repositories are laid out, because that is how the
// harnesses themselves read them. `agents/` naming a persona is the one place
// harnaas reads a word it does not write — the directory name is the
// ecosystem's, and renaming it in the manifest would mean no existing
// repository could be referenced without an object per asset.
var assetTypeDirectories = map[string]AssetType{
	"skills":       AssetTypeSkill,
	"rules":        AssetTypeRule,
	"instructions": AssetTypeInstruction,
	"commands":     AssetTypeCommand,
	"agents":       AssetTypePersona,
}

// DirectoryFor returns the directory segment an asset of this type is inferred
// from, and whether the type is one harnaas recognizes.
//
// It is the inference table read backwards, derived from that table rather than
// written out again, because the two answers have to be the same string: a
// caller creating `.harnaas/rules/` is creating the directory a path is inferred
// from, and a second literal would be correct on the day it was written and is
// exactly the kind of pair nobody re-checks. The table is five entries, so it is
// walked rather than inverted at startup — an inverted copy would be the second
// literal by another route.
func DirectoryFor(assetType AssetType) (string, bool) {
	for directory, candidate := range assetTypeDirectories {
		if candidate == assetType {
			return directory, true
		}
	}
	return "", false
}

// AssetTypes returns every type harnaas recognizes, in the order a diagnostic
// lists them.
//
// The order is the one the manifest's own vocabulary is written in rather than
// alphabetical, and it is fixed so two runs produce the same sentence.
func AssetTypes() []AssetType {
	return []AssetType{
		AssetTypeSkill,
		AssetTypeRule,
		AssetTypeInstruction,
		AssetTypeCommand,
		AssetTypePersona,
	}
}

// AssetRef is an asset entry's source string, parsed into where its content
// comes from and the path to it.
//
// Parsing settles the grammar and containment, and nothing else. It does not
// look the source key up, read the path or infer anything from it, because a
// string that is not one of the two accepted forms has no meaningful key and no
// meaningful path to check further.
type AssetRef struct {
	// SourceKey is the `sources` key the path is relative to. It is empty for
	// the project-local form, which is unambiguous because `sources` rejects an
	// empty key.
	SourceKey string

	// Path is the path to the asset, cleaned and slash-separated: within the
	// source for the keyed form, and relative to the project root — still
	// beginning `.harnaas/` — for the local form.
	Path string
}

// Local reports whether the reference is the project-local form.
func (r AssetRef) Local() bool {
	return r.SourceKey == ""
}

// assetSourceForms states both accepted forms, which is most of the fix for
// every string that is neither.
var assetSourceForms = fmt.Sprintf(
	"An asset source is either %q, naming a key declared in %q, or a project-local path such as %q.",
	"acme:skills/review", "sources", LocalRoot+"/rules/house-style.md",
)

// ParseAssetRef parses one asset entry's source string.
//
// The two accepted forms are distinguished by their prefix rather than by
// trying one and falling back to the other, so a string in neither form is
// reported as what it is instead of as a failed parse of a form its author
// never used.
func ParseAssetRef(index int, source string) (AssetRef, *Violation) {
	switch {
	case source == "":
		return AssetRef{}, assetSourceViolation(index,
			assetSubject(index)+" declares no source",
			fmt.Sprintf("Give the entry a source in %s. %s", FileName, assetSourceForms),
		)

	case isAbsolutePath(source):
		// Named separately from the general "neither form" case because an
		// absolute path is a specific misunderstanding — that harnaas installs
		// from anywhere on the machine — and the fix is to move the content
		// into the repository, not to add a prefix to the same path.
		return AssetRef{}, assetSourceViolation(index,
			fmt.Sprintf("%s names the absolute path %q, and every asset source is relative", assetSubject(index), source),
			fmt.Sprintf(
				"Move the content under %s/ in this repository and reference it from there, or declare its repository in %q. %s",
				LocalRoot, "sources", assetSourceForms,
			),
		)

	case strings.ContainsRune(source, '\\'):
		// A backslash is an ordinary filename character on Linux and a
		// separator on Windows, so a manifest containing one would name two
		// different files depending on who ran install. Rejecting it here, in
		// the grammar, is what keeps the same committed file mean the same
		// thing on every machine that reads it.
		return AssetRef{}, assetSourceViolation(index,
			fmt.Sprintf("%s has the source %q, which separates its path with a backslash", assetSubject(index), source),
			fmt.Sprintf("Write the path in %s with %q separators, as in %q.", FileName, "/", "acme:skills/review"),
		)

	case strings.HasPrefix(source, LocalRoot+"/"):
		return parseLocalRef(index, source)
	}

	key, rest, keyed := strings.Cut(source, ":")
	if !keyed || !validSourceKey(key) || rest == "" || strings.HasPrefix(rest, "/") {
		return AssetRef{}, notAnAssetForm(index, source)
	}

	cleaned := path.Clean(rest)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return AssetRef{}, assetSourceViolation(index,
			fmt.Sprintf("%s has the path %q, which leaves the root of the source %q", assetSubject(index), rest, key),
			fmt.Sprintf("Write the path in %s as one relative to the root of %q, without leading %q segments.", FileName, key, ".."),
		)
	}

	return AssetRef{SourceKey: key, Path: cleaned}, nil
}

// parseLocalRef checks that a project-local path stays under `.harnaas`.
//
// The check is textual and happens before anything is opened, because the
// content of a path that escapes must never be read — not even to report that
// it exists.
func parseLocalRef(index int, source string) (AssetRef, *Violation) {
	cleaned, contained := containedUnderLocalRoot(source)
	if !contained || cleaned == LocalRoot {
		return AssetRef{}, assetSourceViolation(index,
			fmt.Sprintf("%s has the local path %q, which does not stay inside %s", assetSubject(index), source, LocalRoot),
			fmt.Sprintf(
				"Move the content under %s/ and write the path without %q segments, as in %q.",
				LocalRoot, "..", LocalRoot+"/rules/house-style.md",
			),
		)
	}

	return AssetRef{Path: cleaned}, nil
}

// notAnAssetForm reports a source string in neither accepted form.
func notAnAssetForm(index int, source string) *Violation {
	return assetSourceViolation(index,
		fmt.Sprintf("%s has the source %q, which is neither of the two accepted forms", assetSubject(index), source),
		fmt.Sprintf("Rewrite the source in %s. %s", FileName, assetSourceForms),
	)
}

// isAbsolutePath reports whether a source string names a location on the
// machine rather than inside the project or a source.
//
// Both spellings are recognized on every platform: a manifest is committed and
// read on all of them, so a Windows path must be refused when the run happens
// to be on Linux and the other way round.
func isAbsolutePath(source string) bool {
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, `\\`) {
		return true
	}

	if len(source) < 3 || source[1] != ':' {
		return false
	}
	letter := source[0] | 0x20
	return letter >= 'a' && letter <= 'z' && (source[2] == '/' || source[2] == '\\')
}

// CheckSourceKey reports an asset referencing a key the `sources` block does
// not declare.
//
// It is separate from [ParseAssetRef] because the grammar is a property of the
// string and this is a property of the document: the same string is correct in
// one manifest and wrong in another, and only the second failure can name the
// keys that would have worked.
func CheckSourceKey(index int, ref AssetRef, sources map[string]Source) *Violation {
	if ref.Local() {
		return nil
	}
	if _, found := sources[ref.SourceKey]; found {
		return nil
	}

	known := make([]string, 0, len(sources))
	for key := range sources {
		known = append(known, quoted(key))
	}
	slices.Sort(known)

	fix := fmt.Sprintf("Declare %q in the %q block of %s, or correct the key on this entry.", ref.SourceKey, "sources", FileName)
	if len(known) > 0 {
		fix = fmt.Sprintf(
			"Use a key the %q block of %s declares (%s), or declare %q there.",
			"sources", FileName, strings.Join(known, ", "), ref.SourceKey,
		)
	}

	return assetSourceViolation(index,
		fmt.Sprintf("%s references the source key %q, which %q does not declare", assetSubject(index), ref.SourceKey, "sources"),
		fix,
	)
}

// InferType infers an asset's type from the directory containing it.
//
// It is separate from [InferID] because the object form suppresses inference
// one field at a time: an entry declaring `type` for a source laid out
// unconventionally still wants its id inferred from the leaf, and would
// otherwise be refused for a directory name nobody is relying on.
func InferType(index int, ref AssetRef) (AssetType, *Violation) {
	directory := path.Base(path.Dir(ref.Path))

	assetType, named := assetTypeDirectories[directory]
	if !named {
		return "", assetSourceViolation(index,
			fmt.Sprintf(
				"%s has the path %q, whose containing directory names none of the asset types",
				assetSubject(index), ref.Path,
			),
			fmt.Sprintf(
				"Either reference a path under one of %s, or write the entry in %s as an object declaring %q.",
				directoryList(), FileName, "type",
			),
		)
	}

	return assetType, nil
}

// InferID infers an asset's id from its path's leaf, with any file extension
// removed so `tone.md` and `tone` name the same asset.
func InferID(index int, ref AssetRef) (string, *Violation) {
	leaf := path.Base(ref.Path)

	// A leading dot is the extension by path.Ext's reckoning, so stripping
	// unconditionally would turn `.gitignore` into an empty id. Only a
	// non-empty remainder is an id.
	if stripped := strings.TrimSuffix(leaf, path.Ext(leaf)); stripped != "" {
		leaf = stripped
	}

	if leaf == "" || leaf == "." || leaf == ".." {
		return "", assetSourceViolation(index,
			fmt.Sprintf("%s has the path %q, whose last segment is not a name an asset can be identified by", assetSubject(index), ref.Path),
			fmt.Sprintf("Reference the asset's own file or directory, or write the entry in %s as an object declaring %q.", FileName, "id"),
		)
	}

	return leaf, nil
}

// directoryList renders the type-naming directories for a diagnostic, in the
// same fixed order [AssetTypes] uses.
func directoryList() string {
	byType := make(map[AssetType]string, len(assetTypeDirectories))
	for directory, assetType := range assetTypeDirectories {
		byType[assetType] = directory
	}

	names := make([]string, 0, len(byType))
	for _, assetType := range AssetTypes() {
		names = append(names, quoted(byType[assetType]+"/"))
	}
	return strings.Join(names, ", ")
}
