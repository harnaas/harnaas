package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// renderRequest is everything a renderer is allowed to see.
//
// It carries values and no handles: a renderer is a deterministic function of
// the source content and the target, so the same request renders the same bytes
// on any machine and at any time. That is what lets the installed digest be
// compared across machines at all — a renderer that read the clock, the
// environment or the filesystem would make every teammate's install look like
// drift to every other.
type renderRequest struct {
	// Asset is the interpreted manifest entry, which is where the id, the type
	// and the declared source come from.
	Asset manifest.Asset

	// Files is the resolved source content, sorted by path.
	Files []source.File

	// Renderer is the name the adapter's surface declared.
	Renderer adapter.Renderer

	// Target is the harness this rendering is for, so a refusal names the
	// pairing rather than only the asset: the same asset may render natively
	// for one harness and be unsupported on the next.
	Target harness.ID
}

// rendered is what a renderer produces: the bytes to install, and whether doing
// so was an emulation.
type rendered struct {
	// Files is the content to write, relative to the destination and sorted by
	// path.
	Files []source.File

	// Emulated records that this asset reached the harness through a different
	// type's surface. It is never inferred from the renderer name by a caller,
	// because the identity renderer is used both natively and, one day, by a
	// surface that emulates without transforming.
	Emulated bool

	// Note states how the installed form differs from native support, in one
	// sentence, for a report that must never present an emulation as plain
	// success.
	Note string
}

// renderFunc is a renderer's implementation.
type renderFunc func(renderRequest) (*rendered, error)

// renderers maps every renderer harnaas can actually produce to its
// implementation.
//
// It is deliberately smaller than [adapter.Renderers]. The contract names what
// a surface may declare and this names what exists, and the gap between them is
// a supported state rather than an oversight — see [unimplementedRendererError].
var renderers = map[adapter.Renderer]renderFunc{
	adapter.RendererIdentity:      renderIdentity,
	adapter.RendererAsSkill:       renderAsSkill,
	adapter.RendererAsInstruction: renderAsInstruction,
}

// unimplementedRendererError reports a surface that declared a renderer nobody
// has written.
//
// Falling back to the identity renderer is the tempting alternative and the
// wrong one: it would write the source bytes into a path whose harness expects
// a converted format, producing a file that is silently ignored. A tool whose
// purpose is telling a team what is in effect must not do that, so the pairing
// is reported unsupported and nothing is written.
type unimplementedRendererError struct {
	// Renderer is the name the surface declared, and AssetID and Target say
	// which pairing met it.
	Renderer adapter.Renderer
	AssetID  string
	Target   string
}

func (e *unimplementedRendererError) Error() string {
	return fmt.Sprintf(
		"%q needs the %q renderer to install for %s, and harnaas does not implement it\n\n"+
			"Nothing was written for this target. Remove %s from the entry's %q in %s, "+
			"or install this asset for a harness whose surface harnaas can write.",
		e.AssetID, e.Renderer, e.Target, e.Target, "targets", manifest.FileName,
	)
}

// unknownRendererError reports a renderer name the contract does not declare,
// which is a wiring mistake in an adapter rather than anything a user did.
type unknownRendererError struct {
	Renderer adapter.Renderer
}

func (e *unknownRendererError) Error() string {
	names := make([]string, 0, len(adapter.Renderers()))
	for _, name := range adapter.Renderers() {
		names = append(names, fmt.Sprintf("%q", name))
	}
	return fmt.Sprintf(
		"an adapter declared the renderer %q, which the contract does not name (it names %s)",
		e.Renderer, strings.Join(names, ", "),
	)
}

// render runs the renderer a surface declared.
//
// An undeclared renderer name is the empty one, which means the surface said
// nothing: copying is the default rather than the rule, so it renders
// identically. Every other name goes through the table, and a name the contract
// declares but nobody implemented is reported rather than approximated.
func render(request renderRequest) (*rendered, error) {
	name := request.Renderer
	if name == "" {
		name = adapter.RendererIdentity
	}

	if implementation, found := renderers[name]; found {
		return implementation(request)
	}
	if slices.Contains(adapter.Renderers(), name) {
		return nil, &unimplementedRendererError{Renderer: name, AssetID: request.Asset.ID, Target: string(request.Target)}
	}
	return nil, &unknownRendererError{Renderer: name}
}

// renderIdentity reproduces the source bytes exactly.
//
// Every byte survives: frontmatter, line endings and trailing whitespace alike.
// Normalizing any of them would make the installed digest differ from the
// source digest for no reason a reader could see, and would rewrite a `paths:`
// list whose quoting decides what a rule applies to.
func renderIdentity(request renderRequest) (*rendered, error) {
	files := make([]source.File, 0, len(request.Files))
	for _, file := range request.Files {
		files = append(files, source.File{
			Path:    file.Path,
			Content: file.Content,
			Digest:  file.Digest,
		})
	}
	return &rendered{Files: files}, nil
}

// skillFileName is the document a skill is, and the file every emulation
// through the skill surface produces.
const skillFileName = source.SkillFileName

// autoInvokeKey is the frontmatter key that decides whether the harness may
// start a skill on its own initiative.
const autoInvokeKey = "auto-invoke"

// renderAsSkill delivers a single-file asset through the skill surface.
//
// A command is invoked deliberately by a user typing a token for it, and a
// skill is started by the harness when a description matches. Delivering one as
// the other without disabling that would change what the asset *is* — the user
// would find the harness running their deploy command because the conversation
// mentioned deploying — so the disabling line is the whole point of the
// renderer rather than a detail of it.
//
// The frontmatter is edited textually, never re-encoded. harnaas has no YAML
// encoder by design, and one line inserted into the block leaves every other
// key byte-identical, which is exactly what "changing no other frontmatter key"
// has to mean for a document whose quoting carries meaning.
func renderAsSkill(request renderRequest) (*rendered, error) {
	if len(request.Files) != 1 {
		return nil, fmt.Errorf(
			"%q renders through the skill surface, which takes one file, and it resolved to %d",
			request.Asset.ID, len(request.Files),
		)
	}

	content := withFrontmatterKey(request.Files[0].Content, autoInvokeKey, "false")
	return &rendered{
		Files:    []source.File{{Path: skillFileName, Content: content, Digest: source.DigestContent(content)}},
		Emulated: true,
		Note: fmt.Sprintf(
			"installed as a skill because this harness has no %s surface; "+
				"the harness will not invoke it on its own initiative, so it has to be asked for by name",
			request.Asset.Type,
		),
	}, nil
}

// renderAsInstruction delivers a rule as always-on memory-file content.
//
// It applies only to a rule that declares no path scoping, and the caller is
// what enforces that: a scoped rule delivered this way would apply everywhere
// instead of under the paths it named, which is a silent widening of meaning
// and is reported unsupported instead.
func renderAsInstruction(request renderRequest) (*rendered, error) {
	if len(request.Files) != 1 {
		return nil, fmt.Errorf(
			"%q renders into the memory file, which takes one file, and it resolved to %d",
			request.Asset.ID, len(request.Files),
		)
	}

	file := request.Files[0]
	return &rendered{
		Files:    []source.File{{Path: file.Path, Content: file.Content, Digest: file.Digest}},
		Emulated: true,
		Note: "installed into the memory file because this harness has no rules directory; " +
			"it is always on rather than discovered as its own file",
	}, nil
}

// withFrontmatterKey returns content with key set to value in its frontmatter,
// touching nothing else.
//
// Three cases, and none of them re-encodes anything. A block already declaring
// the key has that one line replaced; a block without it gains one line at the
// end; content with no frontmatter at all gains a block holding only this key.
// Everything outside the edited line is copied byte for byte.
func withFrontmatterKey(content []byte, key, value string) []byte {
	line := key + ": " + value

	matter, found := source.SplitFrontmatter(content)
	if !found {
		return []byte("---\n" + line + "\n---\n" + string(content))
	}

	block := matter.Raw
	replaced := false
	lines := splitLinesKeepingEndings(block)
	for i, existing := range lines {
		if !declaresKey(existing, key) {
			continue
		}
		ending := ""
		if bytes.HasSuffix(existing, []byte("\n")) {
			ending = "\n"
		}
		lines[i] = []byte(line + ending)
		replaced = true
		break
	}

	block = bytes.Join(lines, nil)
	if !replaced {
		if len(block) > 0 && block[len(block)-1] != '\n' {
			block = append(block, '\n')
		}
		block = append(block, []byte(line+"\n")...)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(block)
	out.WriteString("---\n")
	out.Write(matter.Body)
	return out.Bytes()
}

// declaresKey reports whether a frontmatter line declares key at the top level.
//
// Leading whitespace disqualifies it: an indented key belongs to the mapping
// above it, and replacing one would move a nested setting to the top level and
// change what the document says.
func declaresKey(line []byte, key string) bool {
	text := strings.TrimRight(string(line), " \t\r\n")
	return strings.HasPrefix(text, key+":") || text == key+":"
}
