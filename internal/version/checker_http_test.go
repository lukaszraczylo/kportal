package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeChecker builds a Checker whose HTTP client is wired to the given test
// server. Because fetchLatestRelease constructs its URL from owner+repo, we
// embed the server's base URL directly in the owner field so the final URL
// becomes "<serverURL>/<repo>/releases/latest" – fine for an httptest server
// that ignores the path.
func makeCheckerWithServer(t *testing.T, srv *httptest.Server, currentVersion string) *Checker {
	t.Helper()
	c := NewChecker("owner", "repo", currentVersion)
	// Replace the HTTP client with one whose transport rewrites every outgoing
	// request to the test server, regardless of the original URL. This is
	// necessary because fetchLatestRelease hard-codes the GitHub API URL, so
	// we cannot influence the host via owner/repo fields.
	c.client = &http.Client{
		Timeout:   5 * time.Second,
		Transport: &rewriteTransport{inner: srv.Client().Transport, base: srv.URL},
	}
	return c
}

// rewriteTransport redirects every outgoing request to baseURL, preserving
// the path and query of the original request.
type rewriteTransport struct {
	inner http.RoundTripper
	base  string
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request and rewrite the host to our test server.
	r2 := req.Clone(req.Context())
	r2.URL.Scheme = "http"
	// Parse just the host from base (strip scheme prefix).
	host := strings.TrimPrefix(rt.base, "http://")
	host = strings.TrimPrefix(host, "https://")
	r2.URL.Host = host
	return rt.inner.RoundTrip(r2)
}

// TestNewChecker verifies the constructor sets fields correctly.
func TestNewChecker_FieldsSet(t *testing.T) {
	c := NewChecker("myowner", "myrepo", "v1.2.3")
	require.NotNil(t, c)
	assert.Equal(t, "myowner", c.owner)
	assert.Equal(t, "myrepo", c.repo)
	assert.Equal(t, "1.2.3", c.current) // normalizeVersion strips the "v"
	assert.NotNil(t, c.client)
}

// TestNewChecker_NormalizesVersion ensures the v-prefix is stripped at construction.
func TestNewChecker_NormalizesVersion(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"v0.1.0", "0.1.0"},
		{"V2.0.0", "2.0.0"},
		{"3.0.0", "3.0.0"},
		{"  v1.0.0  ", "1.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			c := NewChecker("o", "r", tc.input)
			assert.Equal(t, tc.expected, c.current)
		})
	}
}

// TestCheckForUpdate_NewerVersionAvailable verifies an UpdateInfo is returned
// when the server reports a newer tag.
func TestCheckForUpdate_NewerVersionAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName: "v2.0.0",
			HTMLURL: "https://github.com/example/repo/releases/tag/v2.0.0",
			Name:    "Release v2.0.0",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	info := c.CheckForUpdate(context.Background())

	require.NotNil(t, info)
	assert.Equal(t, "1.0.0", info.CurrentVersion)
	assert.Equal(t, "2.0.0", info.LatestVersion)
	assert.Equal(t, "https://github.com/example/repo/releases/tag/v2.0.0", info.ReleaseURL)
	assert.Equal(t, "Release v2.0.0", info.ReleaseName)
}

// TestCheckForUpdate_CurrentIsLatest verifies nil is returned when already on
// the latest version.
func TestCheckForUpdate_CurrentIsLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{TagName: "v1.0.0", HTMLURL: "https://example.com"}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	info := c.CheckForUpdate(context.Background())
	assert.Nil(t, info)
}

// TestCheckForUpdate_CurrentIsNewer verifies nil is returned when the running
// version is ahead of the released one.
func TestCheckForUpdate_CurrentIsNewer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{TagName: "v0.9.0", HTMLURL: "https://example.com"}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	info := c.CheckForUpdate(context.Background())
	assert.Nil(t, info)
}

// TestCheckForUpdate_NetworkError verifies nil is returned on network failure
// (fail-silent contract).
func TestCheckForUpdate_NetworkError(t *testing.T) {
	// Point at a server that is immediately closed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close before the request is made

	c := makeCheckerWithServer(t, srv, "1.0.0")
	info := c.CheckForUpdate(context.Background())
	assert.Nil(t, info, "network error should return nil (fail silent)")
}

// TestCheckForUpdate_CancelledContext verifies nil is returned when the
// context is already cancelled.
func TestCheckForUpdate_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{TagName: "v9.9.9"}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	info := c.CheckForUpdate(ctx)
	assert.Nil(t, info, "cancelled context should return nil")
}

// TestFetchLatestRelease_NonOKStatus verifies an error is returned for non-200
// responses (e.g. rate-limit 403, 404, 500).
func TestFetchLatestRelease_NonOKStatus(t *testing.T) {
	codes := []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError, http.StatusTooManyRequests}
	for _, code := range codes {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			c := makeCheckerWithServer(t, srv, "1.0.0")
			release, err := c.fetchLatestRelease(context.Background())
			assert.Nil(t, release)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "status")
		})
	}
}

// TestFetchLatestRelease_MalformedJSON verifies an error is returned when the
// response body is not valid JSON.
func TestFetchLatestRelease_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	release, err := c.fetchLatestRelease(context.Background())
	assert.Nil(t, release)
	require.Error(t, err)
}

// TestFetchLatestRelease_EmptyTagName verifies that a response with no tag_name
// is parsed (returns a ReleaseInfo with empty TagName) without error.
func TestFetchLatestRelease_EmptyTagName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"html_url":"https://example.com","name":"no tag"}`))
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	release, err := c.fetchLatestRelease(context.Background())
	require.NoError(t, err)
	require.NotNil(t, release)
	assert.Empty(t, release.TagName)
	assert.Equal(t, "https://example.com", release.HTMLURL)
}

// TestFetchLatestRelease_RequestHeaders verifies the Accept and User-Agent
// headers are set on the outgoing request.
func TestFetchLatestRelease_RequestHeaders(t *testing.T) {
	var gotAccept, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotUA = r.Header.Get("User-Agent")
		release := ReleaseInfo{TagName: "v1.0.0"}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "1.0.0")
	_, err := c.fetchLatestRelease(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "application/vnd.github.v3+json", gotAccept)
	assert.Equal(t, "kportal-version-checker", gotUA)
}

// TestCheckForUpdate_WithVPrefix verifies that a tag like "v2.0.0" is
// normalised correctly before comparison.
func TestCheckForUpdate_WithVPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName: "v1.1.0",
			HTMLURL: "https://example.com/v1.1.0",
		}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	c := makeCheckerWithServer(t, srv, "v1.0.0")
	info := c.CheckForUpdate(context.Background())
	require.NotNil(t, info)
	assert.Equal(t, "1.1.0", info.LatestVersion)
	assert.Equal(t, "1.0.0", info.CurrentVersion)
}

// TestParseVersion_EdgeCases covers inputs not exercised by the existing tests.
func TestParseVersion_EdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []int
	}{
		{"empty string", "", []int{0}},
		{"single digit", "3", []int{3}},
		{"non-numeric part", "abc", []int{0}},
		{"mixed numeric and alpha", "1.abc.3", []int{1, 0, 3}},
		{"build metadata only", "1.0.0+meta", []int{1, 0, 0}},
		{"pre-release only", "1.0.0-alpha.1", []int{1, 0, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseVersion(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestIsNewerVersion_EqualLength covers the equal-length tie case.
func TestIsNewerVersion_EqualLength(t *testing.T) {
	// Equal versions with same length: not newer.
	assert.False(t, isNewerVersion("1.2.3", "1.2.3"))
}
