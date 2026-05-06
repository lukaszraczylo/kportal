package httplog

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFlattenHeaders_RedactsSensitive verifies that flattenHeaders replaces
// the values of known sensitive headers with the [REDACTED] placeholder while
// preserving the header name and leaving benign headers untouched. Covers
// the explicit name list, case-insensitive matching, and the substring-based
// fallback patterns ("token", "secret", "password", "apikey").
func TestFlattenHeaders_RedactsSensitive(t *testing.T) {
	h := http.Header{
		// Explicit list (canonical casing)
		"Authorization":       []string{"Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig"},
		"Proxy-Authorization": []string{"Basic dXNlcjpwYXNz"},
		"Cookie":              []string{"session=abc123; csrf=xyz"},
		"Set-Cookie":          []string{"session=abc123; HttpOnly"},
		"X-Api-Key":           []string{"sk_live_deadbeef"},
		"X-Auth-Token":        []string{"tok_supersecret"},
		"X-Csrf-Token":        []string{"csrf_random_value"},
		"X-Access-Token":      []string{"at_anothersecret"},

		// Substring matches (case-insensitive)
		"X-Refresh-Token":  []string{"rt_value"},
		"My-Secret-Header": []string{"shh"},
		"X-User-Password":  []string{"hunter2"},
		"X-Custom-Apikey":  []string{"key_value"},

		// Benign headers must be preserved verbatim
		"Content-Type": []string{"application/json"},
		"Accept":       []string{"text/html", "application/json"},
		"User-Agent":   []string{"kportal-test/1.0"},
	}

	result := flattenHeaders(h)

	redactedHeaders := []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"Set-Cookie",
		"X-Api-Key",
		"X-Auth-Token",
		"X-Csrf-Token",
		"X-Access-Token",
		"X-Refresh-Token",
		"My-Secret-Header",
		"X-User-Password",
		"X-Custom-Apikey",
	}
	for _, name := range redactedHeaders {
		got, ok := result[name]
		assert.Truef(t, ok, "expected redacted header %q to remain present in output", name)
		assert.Equalf(t, "[REDACTED]", got, "expected header %q value to be redacted", name)
	}

	// Benign headers should be untouched.
	assert.Equal(t, "application/json", result["Content-Type"])
	assert.Equal(t, "text/html, application/json", result["Accept"])
	assert.Equal(t, "kportal-test/1.0", result["User-Agent"])

	// And no benign value should leak the redaction marker (sanity check).
	for _, name := range []string{"Content-Type", "Accept", "User-Agent"} {
		assert.NotEqualf(t, "[REDACTED]", result[name], "benign header %q must not be redacted", name)
	}
}

// TestShouldRedactHeader_CaseInsensitive verifies that the case-insensitive
// match logic catches lowercased / mixed-case variants of the redaction list.
func TestShouldRedactHeader_CaseInsensitive(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"authorization", true},
		{"AUTHORIZATION", true},
		{"AuThOrIzAtIoN", true},
		{"cookie", true},
		{"set-cookie", true},
		{"x-api-key", true},
		{"X-CUSTOM-TOKEN", true},
		{"x-app-Secret", true},
		{"My_Password_Header", true},
		{"x-vendor-APIKEY", true},

		// Non-sensitive
		{"Content-Type", false},
		{"Accept", false},
		{"User-Agent", false},
		{"X-Request-Id", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldRedactHeader(tc.name))
		})
	}
}
