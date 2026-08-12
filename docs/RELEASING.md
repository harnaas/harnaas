# Releasing

Pushing a tag matching `v*` is the whole release. [`release.yml`](../.github/workflows/release.yml)
runs `mise run release`, which runs `goreleaser release --clean`, which builds the archives, creates
the GitHub release here, and pushes a Homebrew cask and a Scoop manifest into two other
repositories. Nothing else is manual, and nothing in either of those repositories is edited by hand.

## What a release writes, and where

| Destination | Artifact | Written with |
| --- | --- | --- |
| `harnaas/harnaas` | the release, six archives, `checksums.txt` | `GITHUB_TOKEN` |
| `harnaas/homebrew-tap` | `Casks/harnaas.rb` | `HOMEBREW_TAP_DEPLOY_KEY` |
| `harnaas/scoop-bucket` | `bucket/harnaas.json` | `SCOOP_BUCKET_DEPLOY_KEY` |

Three credentials, because they reach three different places. Actions supplies `GITHUB_TOKEN` on its
own and it is scoped to the repository the workflow runs in — which is exactly why it cannot publish
the cask or the manifest, and why the other two exist.

The archives cover darwin, linux and windows on both amd64 and arm64. The version and commit are
stamped into the binary through `-X` ldflags, so a release build reports its tag from
`harnaas --version` rather than falling back to Go's embedded build information.

## The credentials

Each tap is pushed over SSH with a **write-scoped deploy key of its own**, rather than with one
token granting `Contents: write` on both. That is two properties rather than one:

- A deploy key reaches exactly one repository. The credential that writes the cask cannot write the
  bucket, and neither grants anything else in the organization. A shared token has no such seam.
- A deploy key does not expire. A token does, and the release that first met an expired one would be
  the release that published the archives and silently left both package managers on the previous
  version — the failure that is hardest to notice, because `brew upgrade` simply keeps reporting the
  old version.

Both are already installed. `mise run release` refuses before goreleaser starts if either is
missing, naming the ones that are, so this never degrades into a half-published release.

### The organization policy this depends on

Deploy keys are an organization-level policy, and **new organizations default to disabled** — this
one did, and it was turned on to make the above possible. It lives at
<https://github.com/organizations/harnaas/settings/member_privileges> under **Deploy keys**.

Turning it back off does not merely prevent new keys: it **disables the existing ones**, in every
repository in the organization. Both taps stop accepting pushes and every release afterwards fails
at the publish step. If that policy has to change, migrate to a fine-grained token first — see
[Falling back to a token](#falling-back-to-a-token).

### Rotating or recreating a key

Per tap, substituting the repository and the secret name:

```sh
ssh-keygen -t ed25519 -N '' -f ./k -C goreleaser
gh api -X POST repos/harnaas/homebrew-tap/keys \
  -f title="goreleaser (harnaas/harnaas release workflow)" \
  -f key="$(cat ./k.pub)" -F read_only=false
gh secret set HOMEBREW_TAP_DEPLOY_KEY --repo harnaas/harnaas < ./k
rm -f ./k ./k.pub
```

`read_only=false` is the part that matters — a read-only key authenticates fine and then fails the
push. Pipe the private key into `gh secret set` from the file rather than pasting it, so it never
reaches your shell history. Delete the local copies afterwards; the secret is the only copy needed.

To confirm a key is bound to the repository you think it is:

```sh
ssh -i ./k -o IdentitiesOnly=yes -T git@github.com
# Hi harnaas/homebrew-tap! You've successfully authenticated, ...
```

## Cutting a release

```sh
git tag -a v0.9.0 -m 'v0.9.0'
git push origin v0.9.0
```

Two constraints on the number.

**The first tag must be above `v0.8.42`.** This repository was seeded from the entire.io CLI
carrying that project's tags. They are gone from the remote, but the Go module proxy is immutable,
so `go install …@latest` still resolves to `v0.8.42` — a version whose `go.mod` names a different
module and which contains no `cmd/harnaas`. Only a tag above it takes the module path back. Homebrew
and Scoop are unaffected either way: they resolve release assets, not the module proxy.

**A pre-release suffix changes who sees it.** `v1.0.0-rc.1` is published as a GitHub pre-release, is
not what a bare download URL resolves to, and updates *neither* the cask nor the Scoop manifest —
`release.prerelease: auto` and `skip_upload: auto` key off the same distinction, so the three cannot
disagree. A release candidate is therefore installable by URL and never by `brew install`.

The workflow does not run the test suite; it builds the tag it was given. Tag a commit that has
already gone green on `main`.

## Verifying

```sh
gh run watch --repo harnaas/harnaas
gh release view v0.9.0 --repo harnaas/harnaas
```

Then confirm both package managers actually received a commit — the release succeeding does not by
itself mean they did:

```sh
gh api repos/harnaas/homebrew-tap/commits --jq '.[0].commit.message'
gh api repos/harnaas/scoop-bucket/commits --jq '.[0].commit.message'
```

And end to end, on a machine that has them:

```sh
brew update && brew install --cask harnaas && harnaas --version
scoop update && scoop install harnaas; harnaas --version
```

## When it stops working

The GitHub release is created *before* the two package managers are updated — goreleaser publishes
them last precisely because the cask and the manifest need the release's download URLs — so a
credential that has gone bad leaves a published release, two stale taps, and a red workflow run.
Nothing rolls back.

The failure will be an SSH one, and there are three causes worth telling apart:

- **`Permission denied (publickey)`** — the key was deleted from the tap repository, the secret holds
  a key that no longer matches it, or the organization's deploy key policy was turned off. Check the
  policy first: it disables every existing key at once, so both taps failing together points at it,
  while one tap failing alone points at that repository's key.
- **A push rejected after a successful authentication** — the deploy key is installed read-only.
  Recreate it with `read_only=false`; there is no way to promote one in place.
- **`Host key verification failed`** — the runner did not accept github.com's host key. goreleaser's
  default `ssh_command` passes `-o StrictHostKeyChecking=accept-new`, so this should not occur; if it
  does, something has overridden that command.

The fix is to repair the credential and re-run the failed job for the same tag. No new tag is
needed, and two settings are what make that true rather than aspirational. `release.mode` defaults
to `keep-existing`, so the existing release and its notes survive. And `replace_existing_artifacts`
is set, because the failed run had *already* uploaded every archive before it reached the tap — so
without it the re-run would meet `422 already_exists` on the first archive and die there, never
reaching the tap it was re-run to fix.

### Falling back to a token

If deploy keys ever stop being an option, the token form is a small edit: replace each
`repository.git` block in [`.goreleaser.yaml`](../.goreleaser.yaml) with
`token: "{{ .Env.GORELEASER_TOKEN }}"`, set that one secret, and update the guard in
[`mise-tasks/release`](../mise-tasks/release). The token needs to be a fine-grained one owned by the
`harnaas` **organization** (not a personal account), scoped to `homebrew-tap` and `scoop-bucket`
only, with `Contents: Read and write` and nothing else. Its failure shape is different from the
above: `401 Bad credentials` for an expired token, `403 Resource not accessible by personal access
token` for a missing grant. Both taps are public, so a `404` would not be the shape.

## Checking the config without releasing

`goreleaser check` runs on every CI build through `mise run lint:goreleaser`, and fails on a
deprecated property as well as an invalid one — a release config is otherwise exercised once, on the
day a tag is pushed, which is the worst moment to learn that a property was renamed two minor
versions ago.

To exercise the whole build locally, including the generated cask and manifest, without publishing
anything and **without holding either key**:

```sh
mise exec -- goreleaser release --snapshot --clean
```

The cask lands at `dist/homebrew/Casks/harnaas.rb` and the manifest at `dist/scoop/bucket/harnaas.json`.
Both are what a real release would push, with a snapshot version in place of the tag.

That this works with no credentials is why the keys are read through `envOrDefault` rather than a
bare `{{ .Env.… }}`. A token was resolved when the tap was pushed, which a snapshot never does; a
git `private_key` is resolved when the cask is *written*, which a snapshot does every time — so a
bare lookup fails a credential-free snapshot with `map has no entry for key`. Do not "simplify" it
back.
