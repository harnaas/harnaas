package cli

import (
	"fmt"
	"path"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// exampleAssetID is the id a stand-in asset carries: the one an explanation
// falls back to, and the one the scaffolding's own probe is built with. It is
// never written to disk and never read back.
const exampleAssetID = "example"

// explanationEntry is what one asset-type directory's README says about itself:
// what the type is, what shape its content takes, and an id to show it with.
//
// It is a table rather than five strings because the three answers are one
// thought per type, and a reader comparing two directories is comparing the same
// three sentences.
type explanationEntry struct {
	// Summary is what an asset of this type is, in the vocabulary CONTEXT.md
	// fixes. It is deliberately the same sentence the type constants carry.
	Summary string

	// Shape is what goes in the directory: one file, or one directory per asset.
	Shape string

	// Example is the id the example entry uses. It names something a project
	// plausibly has, so the line reads as an example rather than as a
	// placeholder to search for.
	Example string
}

// explanations is one entry per asset type.
//
// A type missing from here would scaffold a directory that explains nothing, so
// a test asserts the table answers for every type the manifest recognizes.
var explanations = map[manifest.AssetType]explanationEntry{
	manifest.AssetTypeSkill: {
		Summary: "A skill is content a harness loads on its own initiative, when its description " +
			"matches the task at hand.",
		Shape: "One directory per skill, named for the skill, containing a `SKILL.md` whose " +
			"frontmatter `name` is that same id. harnaas refuses a skill whose name and directory " +
			"disagree, because a harness would look for it under the name and never find it.",
		Example: "review",
	},
	manifest.AssetTypeRule: {
		Summary: "A rule is always-on guidance, installed as its own file.",
		Shape:   "One Markdown file per rule, named for the rule.",
		Example: "house-style",
	},
	manifest.AssetTypeInstruction: {
		Summary: "An instruction is always-on guidance concatenated into a managed block in the " +
			"project's committed memory file, so it survives a fresh clone.",
		Shape:   "One Markdown file per instruction, named for the instruction.",
		Example: "tone",
	},
	manifest.AssetTypeCommand: {
		Summary: "A command is content a user invokes deliberately, by name.",
		Shape:   "One Markdown file per command, named for the command.",
		Example: "ship",
	},
	manifest.AssetTypePersona: {
		Summary: "A persona is a delegated worker with its own model and tool budget. The directory " +
			"is named `agents` because that is what the ecosystem calls it.",
		Shape:   "One Markdown file per persona, named for the persona.",
		Example: "reviewer",
	},
}

// explanationFor renders the README one scaffolded directory carries.
//
// The example is built from the directory this is about rather than written out
// per type, so the path in the example and the directory it sits in cannot come
// apart. The closing paragraph is the same in every one of them on purpose: the
// thing most worth knowing about this directory is that nothing in it does
// anything until the manifest says so.
func explanationFor(directory scaffoldDir) string {
	entry, known := explanations[directory.Type]
	if !known {
		// Unreachable from the manifest's own types, and a directory with a
		// heading beats one with no file at all — the file is also what makes
		// the directory survive a clone.
		entry = explanationEntry{Example: exampleAssetID}
	}

	extension := ".md"
	if directory.Type == manifest.AssetTypeSkill {
		// A skill resolves as a directory, so its declared path has no
		// extension to strip.
		extension = ""
	}
	example := path.Join(directory.Path, entry.Example+extension)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", path.Base(directory.Path))
	if entry.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", entry.Summary)
	}
	if entry.Shape != "" {
		fmt.Fprintf(&b, "%s\n\n", entry.Shape)
	}
	fmt.Fprintf(&b, "Declare one in `%s`:\n\n", manifest.FileName)
	fmt.Fprintf(&b, "    \"assets\": [\n      %q\n    ]\n\n", example)
	fmt.Fprintf(&b,
		"The type is inferred from this directory's name, so an entry written that way needs no\n"+
			"`type` field. Nothing here is installed until `%s` declares it, and `harnaas install`\n"+
			"only ever reads from `%s` — it writes into the harness directories instead.\n\n",
		manifest.FileName, manifest.LocalRoot)
	fmt.Fprintf(&b,
		"harnaas created this file once and never reads it. It is yours: edit it, or delete it.\n")

	return b.String()
}
