package source

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve writes body as the whole response, failing the test if the write does
// not complete: a handler that wrote less than it meant to would look exactly
// like the size ceiling truncating, which is the thing under test.
func serve(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	_, err := io.WriteString(w, body)
	assert.NoError(t, err)
}

// localFetcher is the fetcher the suite uses to reach a test server.
//
// Every clause of the rule except the exemption for this machine is still in
// force, which is what keeps the redirect tests below honest: a hop aimed
// anywhere but here is checked exactly as production checks it.
func localFetcher() *Fetcher {
	return newFetcher(destinationRule{allowLocal: true})
}

func TestDestinationRuleRefusesEverythingButAnHTTPSForge(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		url    string
		reason string
	}{
		{name: "plaintext", url: "http://example.com/a.tar.gz", reason: `scheme is "http"`},
		{name: "git protocol", url: "git://example.com/a", reason: `scheme is "git"`},
		{name: "file", url: "file:///etc/passwd", reason: `scheme is "file"`},
		{name: "loopback name", url: "https://localhost:8080/a", reason: "addresses this machine"},
		{name: "reserved localhost label", url: "https://forge.localhost/a", reason: "addresses this machine"},
		{name: "loopback v4", url: "https://127.0.0.1/a", reason: "addresses this machine"},
		{name: "loopback v4 alias", url: "https://127.9.9.9/a", reason: "addresses this machine"},
		{name: "loopback v6", url: "https://[::1]:443/a", reason: "addresses this machine"},
		{name: "unspecified", url: "https://0.0.0.0/a", reason: "addresses this machine"},
		{name: "unparseable", url: "https://example.com/%zz", reason: "cannot tell where it points"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := destinationRule{}.check(tc.url)

			var insecure *InsecureDestinationError
			require.ErrorAs(t, err, &insecure)
			assert.Contains(t, insecure.Reason, tc.reason)
		})
	}
}

func TestDestinationRuleAcceptsAForgeOverHTTPS(t *testing.T) {
	t.Parallel()

	require.NoError(t, destinationRule{}.check("https://codeload.github.com/acme/assets/tar.gz/abc123"))
	// The scheme is compared case-insensitively, because a redirect's Location
	// header is not obliged to spell it the way harnaas would.
	require.NoError(t, destinationRule{}.check("HTTPS://codeload.github.com/acme/assets"))
}

func TestFetchRefusesAnInsecureDestinationBeforeSendingAnything(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// The production fetcher, against a plaintext server on this machine: both
	// halves of the rule refuse it.
	body, err := NewFetcher().Fetch(t.Context(), server.URL, Credential{})

	var insecure *InsecureDestinationError
	require.ErrorAs(t, err, &insecure)
	assert.Nil(t, body)
	assert.Zero(t, requests.Load(), "the request must not be sent at all")
}

func TestFetchRefusesARedirectToAnInsecureDestination(t *testing.T) {
	t.Parallel()

	// A host beyond this machine, so the exemption the suite runs under does not
	// apply and the hop is checked the way production checks it.
	const elsewhere = "http://assets.example.com/skills.tar.gz"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere, http.StatusFound)
	}))
	defer server.Close()

	body, err := localFetcher().Fetch(t.Context(), server.URL, Credential{})

	var insecure *InsecureDestinationError
	require.ErrorAs(t, err, &insecure)
	assert.Equal(t, elsewhere, insecure.URL)
	assert.Nil(t, body)
}

func TestFetchStopsAfterTheRedirectLimit(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer server.Close()

	body, err := localFetcher().Fetch(t.Context(), server.URL, Credential{})

	var tooMany *TooManyRedirectsError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, maxRedirects, tooMany.Limit)
	assert.Nil(t, body)
	// The message says how many redirects were followed, so the count has to be
	// the number that were: the first request, plus one per redirect followed.
	assert.Equal(t, int64(maxRedirects+1), requests.Load())
}

func TestFetchFollowsARedirectWithinTheLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			serve(t, w, "arrived")
			return
		}
		http.Redirect(w, r, "/moved", http.StatusFound)
	}))
	defer server.Close()

	body, err := localFetcher().Fetch(t.Context(), server.URL, Credential{})

	require.NoError(t, err)
	assert.Equal(t, "arrived", string(body))
}

// recordingServer answers every request with body and records the authorization
// header each one carried, so what the fetcher presented is asserted from the
// server's side rather than from the request harnaas built.
func recordingServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, func() []string) {
	t.Helper()

	var (
		mu   sync.Mutex
		seen []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()

		handler(w, r)
	}))
	t.Cleanup(server.Close)

	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return slices.Clone(seen)
	}
}

func TestFetchPresentsACredentialAndSendsNoneWithoutOne(t *testing.T) {
	t.Parallel()

	server, headers := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serve(t, w, "arrived")
	})

	_, err := localFetcher().Fetch(t.Context(), server.URL, Credential{Token: "s3cr3t-token", Origin: "HARNAAS_GITHUB_TOKEN"})
	require.NoError(t, err)

	_, err = localFetcher().Fetch(t.Context(), server.URL, Credential{})
	require.NoError(t, err)

	assert.Equal(t, []string{"Bearer s3cr3t-token", ""}, headers(),
		"an unauthenticated fetch is a request with no authorization at all, not one with an empty one")
}

func TestFetchDropsTheCredentialWhenARedirectLeavesTheService(t *testing.T) {
	t.Parallel()

	// The content host stands in for the signed URL an archive endpoint answers
	// with: it carries its own grant in the URL, so there is no hop that both
	// leaves the service and needs the token.
	content, contentHeaders := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serve(t, w, "arrived")
	})
	api, apiHeaders := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/within" {
			serve(t, w, "arrived")
			return
		}
		http.Redirect(w, r, content.URL+"/signed", http.StatusFound)
	})

	_, err := localFetcher().Fetch(t.Context(), api.URL, Credential{Token: "s3cr3t-token", Origin: "GH_TOKEN"})
	require.NoError(t, err)

	assert.Equal(t, []string{""}, contentHeaders(), "the credential was issued for the service harnaas asked, and a redirect is somebody else's instruction")

	// A hop within the same service keeps it, or an endpoint that redirects to
	// its own canonical path would be unauthenticated for no reason.
	_, err = localFetcher().Fetch(t.Context(), api.URL+"/within", Credential{Token: "s3cr3t-token", Origin: "GH_TOKEN"})
	require.NoError(t, err)

	assert.Equal(t, []string{"Bearer s3cr3t-token", "Bearer s3cr3t-token"}, apiHeaders())
}

func TestSameServiceReadsADefaultPortAsTheOneItStandsFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "identical", a: "https://api.example.com/a", b: "https://api.example.com/b", want: true},
		{name: "default port spelled out", a: "https://api.example.com/a", b: "https://api.example.com:443/b", want: true},
		{name: "case", a: "https://API.example.com/a", b: "https://api.example.com/b", want: true},
		{name: "another host", a: "https://api.example.com/a", b: "https://content.example.com/b", want: false},
		// A second service on one host is still a second service.
		{name: "another port", a: "https://api.example.com/a", b: "https://api.example.com:8443/b", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, err := url.Parse(tc.a)
			require.NoError(t, err)
			b, err := url.Parse(tc.b)
			require.NoError(t, err)

			assert.Equal(t, tc.want, sameService(a, b))
		})
	}
}

func TestFetchFailsRatherThanTruncatingAnOversizedBody(t *testing.T) {
	t.Parallel()

	const limit = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serve(t, w, strings.Repeat("x", limit+1))
	}))
	defer server.Close()

	body, err := localFetcher().fetch(t.Context(), server.URL, Credential{}, limit)

	var tooLarge *ResponseTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	assert.Equal(t, int64(limit), tooLarge.Limit)
	// The point of the failure: nothing partial is handed back, so no caller can
	// extract a truncated archive and record its digest as the source's.
	assert.Nil(t, body)
	assert.Contains(t, tooLarge.Error(), "8 bytes")
}

func TestFetchReadsABodyExactlyAtTheLimit(t *testing.T) {
	t.Parallel()

	const limit = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serve(t, w, strings.Repeat("x", limit))
	}))
	defer server.Close()

	body, err := localFetcher().fetch(t.Context(), server.URL, Credential{}, limit)

	require.NoError(t, err)
	assert.Len(t, body, limit)
}

func TestFetchReportsANonOKStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()

	body, err := localFetcher().Fetch(t.Context(), server.URL, Credential{})

	var status *StatusError
	require.ErrorAs(t, err, &status)
	assert.Equal(t, http.StatusNotFound, status.StatusCode)
	assert.Nil(t, body)
}

func TestFetchReportsAnUnreachableHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := server.URL
	server.Close()

	body, err := localFetcher().Fetch(t.Context(), address, Credential{})

	var fetchErr *FetchError
	require.ErrorAs(t, err, &fetchErr)
	assert.Nil(t, body)
}

func TestFetchKeepsACancelledRunRecognizable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := localFetcher().Fetch(ctx, server.URL, Credential{})

	require.ErrorIs(t, err, context.Canceled)
}

// withCredentials returns rawURL carrying the things a redaction has to remove.
func withCredentials(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	parsed.User = url.UserPassword("harnaas", "s3cr3t-password")
	parsed.RawQuery = "signature=s3cr3t-signature"
	return parsed.String()
}

// assertNoCredentials fails when a message repeats anything withCredentials put
// into the URL.
func assertNoCredentials(t *testing.T, message string) {
	t.Helper()

	assert.NotContains(t, message, "s3cr3t-password")
	assert.NotContains(t, message, "s3cr3t-signature")
	assert.NotContains(t, message, "signature=")
}

func TestFetchFailuresCarryNoCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	// Registered as a cleanup rather than deferred: a parallel subtest resumes
	// after the parent function has returned, so a deferred Close would shut the
	// server down before the case that needs it ran — and this test would still
	// have passed, against an unreachable host instead of a 404.
	t.Cleanup(server.Close)

	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachableURL := unreachable.URL
	unreachable.Close()

	cases := []struct {
		name string
		url  string
		// want is the failure the case is meant to reach, asserted so a server
		// that stopped listening cannot quietly turn one case into the other.
		want any
	}{
		{name: "status", url: withCredentials(t, server.URL), want: new(*StatusError)},
		{name: "unreachable", url: withCredentials(t, unreachableURL), want: new(*FetchError)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := localFetcher().Fetch(t.Context(), tc.url, Credential{})

			require.ErrorAs(t, err, tc.want)
			assertNoCredentials(t, err.Error())
			// The destination still has to be identifiable, or a redaction that
			// removed the whole URL would pass this test and help nobody.
			assert.Contains(t, err.Error(), "127.0.0.1")
		})
	}
}

func TestInsecureDestinationErrorRedactsTheURLItWasGiven(t *testing.T) {
	t.Parallel()

	// Built by hand rather than through a fetch, which is the case the redaction
	// living inside Error() exists to cover.
	err := &InsecureDestinationError{
		URL:    "http://harnaas:s3cr3t-password@assets.example.com/a?signature=s3cr3t-signature",
		Reason: "testing",
	}

	assertNoCredentials(t, err.Error())
	assert.Contains(t, err.Error(), "http://assets.example.com/a")
}
