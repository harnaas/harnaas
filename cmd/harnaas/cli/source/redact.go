package source

import "net/url"

// redactedPlaceholder stands in for a URL harnaas could not parse.
//
// An unparseable string is exactly the one harnaas cannot vouch for: the parts
// that would carry a secret are the parts it failed to identify, so echoing it
// to be helpful is the one case where redaction would silently not happen.
const redactedPlaceholder = "<unparseable url>"

// RedactURL renders a URL with everything secret removed, and is the only
// spelling of a URL that harnaas prints, logs or writes to a file.
//
// Two things go, not one. Userinfo is the obvious credential, and a URL carrying
// a password in it is a form harnaas never builds but a manifest, a redirect or a
// git remote may hand it. The query string goes too, because that is where a
// forge's own credentials live: an archive download redirects to a signed CDN URL
// whose `token=` or `X-Amz-Signature=` grants the bearer the same access the
// request had. A redaction that dropped only userinfo would have looked right and
// leaked the one credential harnaas actually meets.
//
// What is left — scheme, host and path — is what a person reading a failure needs
// in order to tell which fetch failed, and it is the same for every user of the
// same source, which is what makes it safe to write into a shared lockfile.
func RedactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return redactedPlaceholder
	}

	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}
