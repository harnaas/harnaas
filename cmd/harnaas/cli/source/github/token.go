package github

import (
	"os"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// A token is read once per run, in this one place, and travels as a
// [source.Credential] from there.
//
// The chain exists because harnaas is rarely the only tool on the machine that
// needs a GitHub token: `gh` establishes `GH_TOKEN` and an Actions job is handed
// `GITHUB_TOKEN`, so honouring both means a CI job that already authenticates
// needs no harnaas-specific configuration at all. `HARNAAS_GITHUB_TOKEN` comes
// first so a project that needs harnaas to read something the ambient token
// cannot has somewhere to say so without disturbing either of the others.

// tokenEnvVars is the chain, in the order it is consulted.
//
// Order is the whole content of the rule, so it is a list read in sequence
// rather than three checks somebody could reorder without noticing: the first
// variable that is set wins, and no later one is consulted at all.
var tokenEnvVars = []string{"HARNAAS_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}

// resolveCredential reads the token chain, or reports that harnaas holds no
// token.
//
// A variable set to nothing counts as unset. A CI job that exports a token
// conditionally leaves an empty value behind rather than an absent variable, and
// sending an empty bearer token is a request that fails where the same request
// without one would have succeeded on a public repository.
func resolveCredential(getenv func(string) string) source.Credential {
	for _, name := range tokenEnvVars {
		if token := strings.TrimSpace(getenv(name)); token != "" {
			return source.Credential{Token: token, Origin: name}
		}
	}
	return source.Credential{}
}

// ambientCredential reads the chain from the process environment, which is what
// a run outside a test does.
func ambientCredential() source.Credential {
	return resolveCredential(os.Getenv)
}

// describeTokenChain names every variable a token may be supplied through, for
// the diagnostic raised when none of them was.
func describeTokenChain() string {
	switch len(tokenEnvVars) {
	case 0:
		return ""
	case 1:
		return tokenEnvVars[0]
	default:
		return strings.Join(tokenEnvVars[:len(tokenEnvVars)-1], ", ") + " or " + tokenEnvVars[len(tokenEnvVars)-1]
	}
}
