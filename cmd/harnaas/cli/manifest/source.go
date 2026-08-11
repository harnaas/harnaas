package manifest

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// LocalRoot is the directory a project's own asset content lives under,
// relative to the project root.
//
// One directory rather than any path the author likes is what makes a local
// asset auditable: everything harnaas may read out of the repository is in one
// place, and a path that leaves it is a mistake harnaas can name without
// knowing anything about the project's layout.
const LocalRoot = ".harnaas"

// SourceKind names how harnaas obtains a source's content.
//
// The kind travels inside the source string rather than in a field of its own
// so a future kind slots into the one grammar instead of growing a second one.
// Parsing recognizes the kind and stops there — nothing here fetches, resolves
// or stats anything — so a manifest can be validated with no network and no
// filesystem, which is what lets `lint` report a bad source offline.
type SourceKind string

const (
	// SourceKindGitHub is a path within a GitHub repository at a ref.
	SourceKindGitHub SourceKind = "github"

	// SourceKindLocal is a path under the project's own `.harnaas` directory.
	SourceKindLocal SourceKind = "local"
)

// sourceKinds is every registered kind, in the order every message lists them.
//
// A slice rather than a map keeps that order identical on every run, which is
// what makes two people's terminals show the same diagnostic for the same file.
var sourceKinds = []SourceKind{SourceKindGitHub, SourceKindLocal}

// Source is one parsed `sources` entry: where a group of assets comes from,
// declared once and referenced by key.
//
// Nothing in it has been resolved. `Repository` is the text between the kind
// and the ref, checked for shape and not for existence, and `Ref` is the text
// after `@`, not a commit. Turning either into content is `install`'s job, and
// keeping it out of here is what makes validation a pure function of the file.
type Source struct {
	// Key is the short name asset entries reference. It is carried on the
	// parsed source so a diagnostic raised downstream can still name the
	// declaration that caused it.
	Key string

	// Kind is the registered kind, already checked against [sourceKinds].
	Kind SourceKind

	// Repository is what the kind names: an `owner/repository` pair for GitHub,
	// a project-relative directory under `.harnaas` for a local source.
	Repository string

	// Ref is the ref a GitHub source is pinned to, exactly as written. It is
	// empty for a local source, which has nothing to pin.
	Ref string
}

// String renders the source back in the form it was written, so a message can
// quote a parsed source without the caller reassembling it.
func (s Source) String() string {
	value := string(s.Kind) + ":" + s.Repository
	if s.Ref == "" {
		return value
	}
	return value + "@" + s.Ref
}

// ParseSource parses one `sources` value into its kind, repository and ref, and
// resolves nothing.
//
// The key is a parameter rather than something recovered from the value because
// every diagnostic here names it: an author reading "the source is malformed"
// has to find which one, and the map key is the only name the declaration has.
func ParseSource(key, value string) (Source, *Violation) {
	if violation := validateSourceKey(key); violation != nil {
		return Source{}, violation
	}

	kindText, remainder, hasKind := strings.Cut(value, ":")
	if !hasKind || kindText == "" || remainder == "" {
		return Source{}, sourceViolation(key,
			fmt.Sprintf("the source %q is %q, which does not name a source kind", key, value),
			fmt.Sprintf(
				"Write the value of %q in %s as a kind, a repository and a ref, as in %q.",
				key, FileName, "github:acme/assets@v1.2.0",
			),
		)
	}

	repository, ref := splitRef(remainder)

	// Every registered kind is listed, so a kind added to [sourceKinds] without
	// a case here reports itself as unrecognized rather than being parsed by
	// whichever branch happened to be last.
	switch SourceKind(kindText) {
	case SourceKindGitHub:
		return parseGitHubSource(key, repository, ref)
	case SourceKindLocal:
		return parseLocalSource(key, repository, ref)
	default:
		return Source{}, sourceViolation(key,
			fmt.Sprintf("the source %q names the kind %q, which harnaas does not recognize", key, kindText),
			fmt.Sprintf("Change %q in %s to use a kind harnaas knows: %s.", key, FileName, kindList()),
		)
	}
}

// parseGitHubSource checks the two things a GitHub source must state: which
// repository, and which ref.
func parseGitHubSource(key, repository, ref string) (Source, *Violation) {
	owner, name, hasOwner := strings.Cut(repository, "/")
	if !hasOwner || owner == "" || name == "" || strings.Contains(name, "/") {
		return Source{}, sourceViolation(key,
			fmt.Sprintf("the source %q names the repository %q, which is not an owner and a repository", key, repository),
			fmt.Sprintf(
				"Write the repository of %q in %s as owner/repository, as in %q.",
				key, FileName, "github:acme/assets@v1.2.0",
			),
		)
	}

	// A GitHub source with no ref is an unpinned dependency, and a manifest
	// whose whole purpose is declaring which version of somebody else's content
	// this repository trusts cannot have one. Defaulting to a branch would make
	// two installs of the same manifest produce different files.
	if ref == "" {
		return Source{}, sourceViolation(key,
			fmt.Sprintf("the source %q declares no ref, so it names no particular version of %q", key, repository),
			fmt.Sprintf(
				"Pin %q in %s to a tag or commit with an @ suffix, as in %q.",
				key, FileName, "github:acme/assets@v1.2.0",
			),
		)
	}

	return Source{Key: key, Kind: SourceKindGitHub, Repository: repository, Ref: ref}, nil
}

// parseLocalSource checks that a local source names a directory inside the
// project's `.harnaas`, and that it pins nothing.
func parseLocalSource(key, repository, ref string) (Source, *Violation) {
	if ref != "" {
		return Source{}, sourceViolation(key,
			fmt.Sprintf("the source %q is local and declares the ref %q, and content in this repository has no ref to be at", key, ref),
			fmt.Sprintf("Remove the %q suffix from %q in %s.", "@"+ref, key, FileName),
		)
	}

	cleaned, contained := containedUnderLocalRoot(repository)
	if !contained {
		return Source{}, sourceViolation(key,
			fmt.Sprintf("the source %q names the local path %q, which is not inside %s", key, repository, LocalRoot),
			fmt.Sprintf(
				"Point %q in %s at a directory under %s/, relative to the repository root.",
				key, FileName, LocalRoot,
			),
		)
	}

	return Source{Key: key, Kind: SourceKindLocal, Repository: cleaned}, nil
}

// validateSourceKey rejects a key an asset entry could not reference.
//
// The asset string grammar splits on the first colon, so a key containing one
// could never be written on an asset; a key containing a slash would be
// indistinguishable from a path. An empty key is rejected because the parsed
// asset reference uses an empty source key to mean the project-local form.
func validateSourceKey(key string) *Violation {
	switch {
	case key == "":
		return sourceViolation(key,
			"the "+quoted("sources")+" block declares an entry with an empty key",
			fmt.Sprintf("Give every entry in %s a short name, such as %q.", quoted("sources"), "acme"),
		)
	case !validSourceKey(key):
		return sourceViolation(key,
			fmt.Sprintf("the source key %q contains a character an asset entry could not reference it by", key),
			fmt.Sprintf(
				"Rename the key in %s to letters, digits, %q, %q or %q, such as %q.",
				FileName, "-", "_", ".", "acme",
			),
		)
	}
	return nil
}

// validSourceKey reports whether a key is one an asset entry could reference,
// which is the same question [validateSourceKey] answers with a diagnostic. The
// asset grammar asks it about text that may not be a key at all, where building
// a violation to discard would be the wrong shape.
func validSourceKey(key string) bool {
	return key != "" && !strings.ContainsAny(key, ":/\\ ")
}

// splitRef separates a ref from what precedes it, taking the last `@` so a
// repository containing one keeps it.
func splitRef(remainder string) (repository, ref string) {
	at := strings.LastIndex(remainder, "@")
	if at < 0 {
		return remainder, ""
	}
	return remainder[:at], remainder[at+1:]
}

// containedUnderLocalRoot reports whether a project-relative path stays inside
// `.harnaas`, returning it in cleaned form.
//
// The check runs on the text, before anything is opened, because a path that
// escapes must never be read — not even to discover that it exists. Cleaning
// first is what makes `.harnaas/../../etc` fail here rather than resolve
// somewhere it should not.
//
// A backslash is rejected outright rather than treated as a separator: it is an
// ordinary filename character on Linux and a separator on Windows, so a path
// containing one would mean two different things depending on who ran install.
func containedUnderLocalRoot(candidate string) (string, bool) {
	if candidate == "" || strings.ContainsRune(candidate, '\\') || strings.HasPrefix(candidate, "/") {
		return "", false
	}

	cleaned := path.Clean(candidate)
	if cleaned == LocalRoot || strings.HasPrefix(cleaned, LocalRoot+"/") {
		return cleaned, true
	}
	return "", false
}

// kindList renders the registered kinds for a diagnostic.
func kindList() string {
	names := make([]string, 0, len(sourceKinds))
	for _, kind := range sourceKinds {
		names = append(names, strconv.Quote(string(kind)))
	}
	return strings.Join(names, ", ")
}

// quoted renders a field or block name the way every diagnostic quotes one.
func quoted(name string) string {
	return strconv.Quote(name)
}
