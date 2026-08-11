package source

import (
	"fmt"
	"path"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// SkillFileName is the document every skill is: the file a harness opens to
// learn what the skill is for. A skill is a directory because it may carry
// references beside that file, but this is the one file it cannot be without.
const SkillFileName = "SKILL.md"

// Verify checks a resolved source against the shape and the naming its asset
// type requires, and is the last thing that happens before an install is
// planned.
//
// It sits here rather than in each kind because it is a question about the
// content and not about where the content came from: a skill fetched from a
// forge and a skill read out of `.harnaas` are wrong in exactly the same way,
// and two implementations of one rule would eventually be two rules. Every kind
// therefore resolves, and this verifies what resolved.
//
// Nothing here reads or writes a file, and nothing rewrites content. That is the
// point of the name check in particular: a skill whose frontmatter `name`
// disagrees with its id is dropped in silence by the harnesses that read that
// field, and harnaas refuses it rather than correcting it, because the bytes
// belong to their author.
func Verify(req Request, resolved *Resolved) error {
	if req.Asset.Type == manifest.AssetTypeSkill {
		return verifySkill(req, resolved)
	}
	return verifySingleFile(req, resolved)
}

// verifySkill holds a skill to being a directory carrying a SKILL.md whose
// frontmatter names it.
func verifySkill(req Request, resolved *Resolved) error {
	if fromSingleFile(req.Asset.Ref.Path, resolved.Files) {
		return &ShapeError{
			AssetID:  req.Asset.ID,
			Type:     req.Asset.Type,
			Expected: "a directory containing " + SkillFileName,
			Found:    "a single file",
		}
	}

	skill, present := fileAt(resolved.Files, SkillFileName)
	if !present {
		return &MissingSkillFileError{AssetID: req.Asset.ID, Source: req.Asset.Ref.Path}
	}

	return verifySkillName(req.Asset.ID, skill.Content)
}

// verifySingleFile holds every other type to being one regular file.
//
// A rule, an instruction, a command and a persona are each one document, and a
// source naming a directory of them is an asset entry that meant to name one of
// its files. Reporting how many were found is what turns "this is wrong" into
// "you pointed at the directory".
func verifySingleFile(req Request, resolved *Resolved) error {
	if fromSingleFile(req.Asset.Ref.Path, resolved.Files) {
		return nil
	}

	return &ShapeError{
		AssetID:  req.Asset.ID,
		Type:     req.Asset.Type,
		Expected: "a single file",
		Found:    describeDirectory(len(resolved.Files)),
	}
}

// verifySkillName compares the id harnaas inferred against the name the skill
// declares for itself.
//
// Both failures below are read-only and neither is a warning: a harness that
// reads the `name` field uses it to decide the skill exists, so a mismatch is an
// asset that installs, reports success and is never invoked.
func verifySkillName(assetID string, content []byte) error {
	block, present := SplitFrontmatter(content)
	if !present {
		return &SkillFrontmatterError{AssetID: assetID, Reason: "it has no frontmatter block"}
	}

	var declared struct {
		Name string `yaml:"name"`
	}
	if err := block.Decode(&declared); err != nil {
		return &SkillFrontmatterError{
			AssetID: assetID,
			Reason:  "its frontmatter is not valid YAML",
			Err:     err,
		}
	}

	if declared.Name == "" {
		return &SkillFrontmatterError{AssetID: assetID, Reason: "its frontmatter declares no name"}
	}
	if declared.Name != assetID {
		return &SkillNameMismatchError{AssetID: assetID, DeclaredName: declared.Name}
	}

	return nil
}

// fromSingleFile reports whether a resolved source came from one file rather
// than from a directory.
//
// Both kinds resolve a single file under its own leaf name, because the path
// relative to a file is nothing — so one file whose path is the leaf of the
// declared subtree is a file, and everything else is a directory. Deriving the
// answer here rather than carrying it out of extraction keeps the shape a
// property of what resolved, which is what every kind already agrees on, instead
// of a flag each kind sets and one of them eventually sets wrongly.
//
// It has one blind spot, stated rather than hidden: a directory holding exactly
// one file named after the directory itself reads as a file. Every such source
// is refused either way — it carries no SKILL.md if it is a skill, and it is one
// file if it is not — so the cost is a message naming the wrong one of two
// shapes for a case no layout produces on purpose.
func fromSingleFile(subtree string, files []File) bool {
	if subtree == "" || len(files) != 1 {
		return false
	}
	return files[0].Path == path.Base(subtree)
}

// fileAt returns the resolved file at a path within the source.
func fileAt(files []File, name string) (File, bool) {
	for _, file := range files {
		if file.Path == name {
			return file, true
		}
	}
	return File{}, false
}

// describeDirectory says what was found where one file was required, in a count
// a reader can check against the directory they are looking at.
func describeDirectory(files int) string {
	if files == 1 {
		return "a directory holding 1 file"
	}
	return fmt.Sprintf("a directory holding %d files", files)
}
