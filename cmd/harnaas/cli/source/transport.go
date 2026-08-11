package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// The transport rules live beside the source contract rather than inside the one
// kind that fetches today, because they are a property of harnaas and not of a
// kind. A second forge added later must not be able to make a request that skips
// them, and the only structural way to guarantee that is for there to be one
// fetcher and no exported way to build another.

// maxRedirects bounds a redirect chain.
//
// An archive download legitimately redirects — a forge answers with a location on
// its content host, which may itself redirect to a CDN — so the limit is not one.
// It is small because every additional hop is another destination nobody declared,
// and a chain longer than a handful is a misconfiguration or a loop rather than a
// service that needs more room.
const maxRedirects = 5

// fetchTimeout bounds a single fetch end to end, including the redirect chain and
// the body read.
//
// The context already ends a run the user interrupted; this ends one nobody is
// watching. A host that accepts a connection and then stalls forever would
// otherwise hang a CI job for as long as the job's own limit allows, which is the
// failure that looks like harnaas being broken rather than the network being.
const fetchTimeout = 5 * time.Minute

// maxResponseBytes is the ceiling on a fetched response body.
//
// Harness assets are documents, and the archive of a repository that holds them
// is measured in megabytes. The ceiling exists so that a URL serving an endless
// stream — a misconfigured proxy, an error page that never ends — fails instead
// of filling the disk, and it is generous enough that a legitimate asset
// repository never meets it. The limit on what an archive expands to is a
// separate and much smaller number, because a compressed stream this size can
// expand to far more.
const maxResponseBytes int64 = 64 << 20

// errBodyTooLarge reports a response body past the ceiling. It is a sentinel so
// [Fetcher.fetch] can tell it from a read that failed and name the limit.
var errBodyTooLarge = errors.New("response body exceeds the limit")

// authorizationHeader carries a [Credential] on a request, and is spelled in one
// place so the header the fetcher sets and the header a redirect drops cannot
// disagree about which one holds the secret.
const authorizationHeader = "Authorization"

// destinationRule decides whether harnaas may send a request to a URL.
//
// It is a value rather than a package-level function because of what it has to
// permit under test and must never permit in production: a test server binds a
// loopback address and speaks plaintext, so a suite that could not talk to one
// could not exercise the redirect limit, the size ceiling or a single successful
// fetch. The exemption is exactly one thing, is unexported, and leaves every other
// clause of the rule in force — including on the redirect hops, which is where the
// rule earns its keep.
type destinationRule struct {
	// allowLocal permits a destination on this machine, which no production
	// fetch may have and every test server is.
	allowLocal bool
}

// check reports why harnaas will not fetch rawURL, or nil if it will.
func (r destinationRule) check(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &InsecureDestinationError{URL: rawURL, Reason: "harnaas cannot tell where it points"}
	}

	if r.allowLocal && addressesThisMachine(parsed.Hostname()) {
		return nil
	}

	if !strings.EqualFold(parsed.Scheme, "https") {
		return &InsecureDestinationError{
			URL:    rawURL,
			Reason: fmt.Sprintf("its scheme is %q rather than https, so anyone on the path could replace what arrives", parsed.Scheme),
		}
	}
	if addressesThisMachine(parsed.Hostname()) {
		return &InsecureDestinationError{
			URL:    rawURL,
			Reason: "it addresses this machine rather than a forge",
		}
	}
	return nil
}

// addressesThisMachine reports whether host names the machine harnaas is running
// on.
//
// A source is somebody else's content, pinned by commit, and content that already
// lives on this machine is what a `local` source under .harnaas is for. So a
// destination here is either a misconfiguration or a redirect aimed at a service
// bound to loopback that never expected a request from the outside — which is the
// reason the check runs on every hop and not only on the URL harnaas built.
//
// The unspecified address is included because connecting to it reaches a local
// service on the platforms that permit it at all, and `.localhost` names because
// RFC 6761 reserves the whole label for exactly this meaning.
func addressesThisMachine(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.IsLoopback() || addr.IsUnspecified()
	}
	return false
}

// Fetcher is the only way harnaas makes an HTTP request.
//
// One type holds every transport rule — the destination check, the redirect
// bound, the timeout and the size ceiling — so that a kind added later inherits
// all of them by having no other way to fetch anything.
type Fetcher struct {
	client *http.Client
	rule   destinationRule
}

// NewFetcher returns the fetcher production uses.
func NewFetcher() *Fetcher {
	return newFetcher(destinationRule{})
}

// newFetcher builds a fetcher under a given destination rule, which is the seam
// the suite needs in order to reach a server at all. See [destinationRule].
func newFetcher(rule destinationRule) *Fetcher {
	return &Fetcher{
		rule: rule,
		client: &http.Client{
			Timeout: fetchTimeout,
			// The destination rule runs again on every hop. Without this the
			// https-only rule is decoration: Go's default policy follows up to
			// ten redirects across schemes and hosts, and only the URL harnaas
			// built was ever checked, so an https entry point could deliver the
			// archive over plaintext from anywhere.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// via holds the requests already made, so it reaches the limit
				// on the hop after the last one harnaas was willing to follow —
				// which is what makes the message's count the number it
				// actually followed rather than one more than that.
				if len(via) > maxRedirects {
					return &TooManyRedirectsError{URL: via[0].URL.String(), Limit: maxRedirects}
				}
				if err := rule.check(req.URL.String()); err != nil {
					return err
				}
				// The credential was issued for the service harnaas asked, and
				// a redirect is somebody else's instruction to ask a different
				// one. Go's own policy already drops the header across
				// unrelated domains, but the rule harnaas needs is narrower
				// than the one it inherits: an archive endpoint answers with a
				// signed URL on a content host that carries its own grant, so
				// there is never a hop that both leaves the service and needs
				// the token.
				if !sameService(via[0].URL, req.URL) {
					req.Header.Del(authorizationHeader)
				}
				return nil
			},
		},
	}
}

// sameService reports whether two destinations are the same host and port, with
// each scheme's default port filled in so that one URL spelling it and one
// leaving it out are not read as two services.
func sameService(a, b *url.URL) bool {
	return strings.EqualFold(a.Hostname(), b.Hostname()) && servicePort(a) == servicePort(b)
}

// servicePort is a URL's port, defaulted from its scheme.
//
// A port change is a service change here, deliberately: a second service on one
// host is still a second service, and the only credential harnaas holds is one
// it was given for the endpoint it asked.
func servicePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "http") {
		return "80"
	}
	return "443"
}

// Fetch retrieves the whole body at rawURL, presenting credential where there is
// one, or fails.
//
// The credential is a parameter rather than a second method, so that every fetch
// answers the question of what it is authenticated as — the unauthenticated
// request is the zero [Credential] rather than a call that forgot to say.
//
// It returns bytes rather than a stream on purpose: the size ceiling is the only
// thing standing between an endless response and the disk, and a caller handed a
// reader would have to honour that ceiling itself. Buffering also makes "no
// partial body ever reaches a caller" structural — a body that failed halfway is
// never returned at all, so a truncated archive cannot be extracted and recorded
// as a source that legitimately resolved to less than it should have.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string, credential Credential) ([]byte, error) {
	return f.fetch(ctx, rawURL, credential, maxResponseBytes)
}

// fetch is [Fetcher.Fetch] with the ceiling as a parameter, so the failure past
// it can be exercised without serving 64 MiB.
func (f *Fetcher) fetch(ctx context.Context, rawURL string, credential Credential, limit int64) ([]byte, error) {
	// Checked before the request is built, so a refused destination is a
	// destination nothing was ever sent to.
	if err := f.rule.check(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &FetchError{URL: rawURL, Err: err}
	}
	if credential.Present() {
		req.Header.Set(authorizationHeader, "Bearer "+credential.Token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, refusalOrFetchError(rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{URL: rawURL, StatusCode: resp.StatusCode, Status: resp.Status}
	}

	body, err := readWithinLimit(resp.Body, limit)
	switch {
	case errors.Is(err, errBodyTooLarge):
		return nil, &ResponseTooLargeError{URL: rawURL, Limit: limit}
	case err != nil:
		return nil, &FetchError{URL: rawURL, Err: err}
	}
	return body, nil
}

// readWithinLimit reads r to its end, or fails reporting that it did not.
//
// One byte past the limit is read deliberately. A bare [io.LimitReader] stops at
// the limit and reports success, so an oversized body arrives truncated and
// indistinguishable from a complete one — which for an archive means a corrupt
// source whose digest is recorded as though it were the real content.
func readWithinLimit(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, errBodyTooLarge
	}
	return body, nil
}

// refusalOrFetchError surfaces a refusal the redirect policy raised as itself.
//
// The client wraps whatever [http.Client.CheckRedirect] returned in a
// [url.Error], whose own message restates the URL — the unredacted one. Returning
// harnaas's error rather than the wrapper keeps the message the user reads the
// finished diagnostic the rule wrote, and keeps the credential out of it.
func refusalOrFetchError(rawURL string, err error) error {
	var insecure *InsecureDestinationError
	if errors.As(err, &insecure) {
		return insecure
	}

	var tooMany *TooManyRedirectsError
	if errors.As(err, &tooMany) {
		return tooMany
	}

	return &FetchError{URL: rawURL, Err: err}
}

// InsecureDestinationError reports a fetch harnaas refused to make because of
// where it pointed, whether that was the URL it built or somewhere a redirect
// sent it.
type InsecureDestinationError struct {
	// URL is the refused destination, held exactly as it was written and
	// redacted where the message is built. Holding the raw value and redacting
	// at the one place it renders is what makes redaction a property of the type
	// rather than a step every caller constructing one has to remember.
	URL string

	// Reason completes "refused ... because ...", naming which clause of the
	// rule the destination failed.
	Reason string
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *InsecureDestinationError) Error() string {
	return fmt.Sprintf(
		"refusing to fetch %s because %s\n\n"+
			"harnaas fetches a source over https from a host other than this machine. "+
			"Declare a source it can reach that way, or put the content in a directory under .harnaas and declare it as a local source.",
		RedactURL(e.URL), e.Reason,
	)
}

// TooManyRedirectsError reports a chain that never arrived anywhere.
type TooManyRedirectsError struct {
	// URL is where the chain started, held raw and redacted where it renders.
	URL string

	// Limit is the number of redirects harnaas followed before stopping.
	Limit int
}

// Error states the problem and then the fix.
func (e *TooManyRedirectsError) Error() string {
	return fmt.Sprintf(
		"fetching %s followed %d redirects without arriving\n\n"+
			"The host is redirecting in a loop or through more hops than harnaas follows. "+
			"Check that the manifest names the repository directly rather than a URL that forwards to it.",
		RedactURL(e.URL), e.Limit,
	)
}

// ResponseTooLargeError reports a body past the size ceiling.
type ResponseTooLargeError struct {
	// URL is the destination that served it, held raw and redacted where it
	// renders.
	URL string

	// Limit is the ceiling in bytes, so the message states the number the
	// response has to come under rather than only that it did not.
	Limit int64
}

// Error states the problem and then the fix.
func (e *ResponseTooLargeError) Error() string {
	return fmt.Sprintf(
		"%s served more than the %d bytes harnaas reads from one source\n\n"+
			"Harness assets are documents, so a source this large is nearly always a repository that happens to contain them rather than the repository they live in. "+
			"Declare a source that holds the assets themselves.",
		RedactURL(e.URL), e.Limit,
	)
}

// StatusError reports a response harnaas will not read.
//
// It is deliberately a plain transport diagnostic: the kind that made the request
// is the only thing that knows which asset, repository and commit the URL stood
// for, so it is what turns a 404 into a message about a missing path.
type StatusError struct {
	// URL is the destination, held raw and redacted where it renders.
	URL string

	// StatusCode is the response status, kept as a number so a caller can
	// recognize the cases it has something better to say about.
	StatusCode int

	// Status is the status line, which carries the host's own wording.
	Status string
}

// Error states the problem and then the fix.
func (e *StatusError) Error() string {
	return fmt.Sprintf(
		"%s answered %s\n\n"+
			"Check that the source the manifest declares exists and is readable, then run harnaas install again.",
		RedactURL(e.URL), e.Status,
	)
}

// FetchError reports a request that produced no usable response at all.
type FetchError struct {
	// URL is the destination, held raw and redacted where it renders.
	URL string

	// Err is the underlying failure, kept so a caller can recognize a cancelled
	// run through errors.Is.
	Err error
}

// Error states the problem and then the fix.
//
// The cause is unwrapped out of its [url.Error] first. That wrapper's own message
// restates the URL unredacted, so quoting it would put back exactly what
// [RedactURL] exists to remove — and it adds nothing, since the destination is
// already the first thing this message names.
func (e *FetchError) Error() string {
	cause := e.Err

	var wrapped *url.Error
	if errors.As(cause, &wrapped) {
		cause = wrapped.Err
	}

	return fmt.Sprintf(
		"could not fetch %s: %s\n\n"+
			"Check that this machine can reach the host, then run harnaas install again.",
		RedactURL(e.URL), cause,
	)
}

// Unwrap keeps the cause reachable, so a fetch ended by an interrupt is still
// recognizable as a cancelled context rather than as a network failure.
func (e *FetchError) Unwrap() error { return e.Err }
