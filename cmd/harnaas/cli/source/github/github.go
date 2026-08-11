// Package github resolves `github` sources: the ref a manifest pinned becomes
// the commit it names, and that commit's repository archive becomes the files
// an asset installs.
//
// Names are resolved with Git and bytes are moved over HTTPS, and the split is
// the whole design. `git ls-remote` goes through whatever credential helper the
// user already configured, so a private repository resolves without harnaas
// inventing an authentication story of its own, and that path has no API rate
// limit for a CI job to exhaust. The archive endpoint is then one request per
// repository and commit rather than one request per file, which is what makes a
// skill — a directory of documents — cost the same as a rule.
//
// Resolution is kept separate from retrieval for a second reason: the lockfile
// records what was *asked for* alongside what it *resolved to*, because "the
// installed files still match the commit" and "the tag now points somewhere
// else" are two different questions and `lint` asks them separately.
package github

// remoteHost is the forge this kind speaks to, and the only place its name is
// spelled: a message, a Git remote and an archive URL all read it from here, so
// they cannot come to disagree about which host a `github` source means.
const remoteHost = "github.com"

// remoteURL is the Git remote for an `owner/repository` pair.
//
// It is built rather than taken from the manifest, which is what keeps a
// declared source from naming a host harnaas never agreed to talk to. The
// repository arrives already checked for shape by the manifest grammar, so this
// is assembly and not validation.
func remoteURL(repository string) string {
	return "https://" + remoteHost + "/" + repository + ".git"
}
