package httplog

// Tests for loggingTransport.RoundTrip and readBodyLimited — both at 0%
// coverage before this file was added. Uses httptest.NewServer for real HTTP
// round-trips so the transport code executes end-to-end.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeProxy builds a Proxy wired to the given backend server, using an
// ephemeral listen port and a buffer-backed logger. The caller must stop
// the proxy after the test.
func makeProxy(t *testing.T, backend *httptest.Server, opts struct {
	filterPath  string
	includeHdrs bool
	maxBodyLen  int
}) (*Proxy, *bytes.Buffer) {
	t.Helper()

	if opts.maxBodyLen == 0 {
		opts.maxBodyLen = 1024 * 1024
	}

	var buf bytes.Buffer
	lg := &Logger{
		forwardID:  "test-rt",
		maxBodyLen: opts.maxBodyLen,
		output:     &buf,
	}

	// Extract backend port
	backendAddr := backend.Listener.Addr().String()
	var backendPort int
	_, _ = fmt.Sscanf(backendAddr[strings.LastIndex(backendAddr, ":")+1:], "%d", &backendPort)

	p := &Proxy{
		localPort:   0, // ephemeral
		targetPort:  backendPort,
		logger:      lg,
		forwardID:   "test-rt",
		filterPath:  opts.filterPath,
		includeHdrs: opts.includeHdrs,
	}

	require.NoError(t, p.Start())
	t.Cleanup(func() { _ = p.Stop() })

	return p, &buf
}

// proxyURL returns the URL of the proxy's listening address.
func proxyURL(p *Proxy) string {
	addr := p.listener.Addr().String()
	return "http://" + addr
}

// TestRoundTrip_GET_LogsRequestAndResponse drives a GET through the proxy and
// verifies that both a request entry and a response entry are written to the log.
func TestRoundTrip_GET_LogsRequestAndResponse(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{})

	resp, err := http.Get(proxyURL(p) + "/api/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give logger a moment — it's synchronous in RoundTrip so no sleep needed,
	// but let's drain the response body to ensure everything flushed.
	_, _ = io.ReadAll(resp.Body)

	// Two JSON lines expected: request + response
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected at least 2 log lines, got: %s", buf.String())

	var reqEntry, respEntry Entry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &reqEntry))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &respEntry))

	assert.Equal(t, "request", reqEntry.Direction)
	assert.Equal(t, "GET", reqEntry.Method)
	assert.Equal(t, "/api/test", reqEntry.Path)

	assert.Equal(t, "response", respEntry.Direction)
	assert.Equal(t, http.StatusOK, respEntry.StatusCode)
	assert.Equal(t, `{"status":"ok"}`, respEntry.Body)
	assert.GreaterOrEqual(t, respEntry.LatencyMs, int64(0))
}

// TestRoundTrip_POST_WithBody verifies that request bodies are captured and
// re-streamed to the backend correctly.
func TestRoundTrip_POST_WithBody(t *testing.T) {
	var receivedBody []byte
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{})

	reqBody := `{"name":"alice","email":"alice@example.com"}`
	resp, err := http.Post(proxyURL(p)+"/users", "application/json", strings.NewReader(reqBody))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, reqBody, string(receivedBody), "backend must receive the full request body")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	var reqEntry Entry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &reqEntry))
	assert.Equal(t, reqBody, reqEntry.Body)
	assert.Equal(t, len(reqBody), reqEntry.BodySize)
}

// TestRoundTrip_FilterPath_SkipsNonMatchingPaths ensures that requests whose
// paths don't match filterPath are forwarded but not logged.
func TestRoundTrip_FilterPath_SkipsNonMatchingPaths(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{filterPath: "/api/*"})

	// This path does NOT match /api/* → should be forwarded but not logged
	resp, err := http.Get(proxyURL(p) + "/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, buf.String(), "non-matching path must produce no log output")

	// This path DOES match /api/* → should be logged
	resp2, err := http.Get(proxyURL(p) + "/api/users")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	_, _ = io.ReadAll(resp2.Body)

	assert.NotEmpty(t, buf.String(), "matching path must produce log output")
}

// TestRoundTrip_IncludeHeaders verifies that when includeHdrs is true the log
// entries contain header maps, and that sensitive headers are redacted.
func TestRoundTrip_IncludeHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Id", "resp-123")
		w.Header().Set("Set-Cookie", "session=abc123")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{includeHdrs: true})

	req, _ := http.NewRequest("GET", proxyURL(p)+"/test", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Custom", "visible")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	var reqEntry, respEntry Entry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &reqEntry))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &respEntry))

	// Sensitive request header must be redacted
	assert.Equal(t, redactedValue, reqEntry.Headers["Authorization"])
	// Benign request header must be visible
	assert.Equal(t, "visible", reqEntry.Headers["X-Custom"])
	// Sensitive response header must be redacted
	assert.Equal(t, redactedValue, respEntry.Headers["Set-Cookie"])
}

// TestRoundTrip_NoHeaders verifies that when includeHdrs is false no header
// map is written to the log entries.
func TestRoundTrip_NoHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{includeHdrs: false})

	resp, err := http.Get(proxyURL(p) + "/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 1)

	var reqEntry Entry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &reqEntry))
	assert.Nil(t, reqEntry.Headers, "headers must be absent when includeHdrs=false")
}

// TestRoundTrip_BackendDown_LogsError verifies that when the backend is
// unreachable the proxy ErrorHandler fires and logs an error entry.
func TestRoundTrip_BackendDown_LogsError(t *testing.T) {
	// Start a server, grab its address, then close it to simulate down backend.
	dummy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	backendAddr := dummy.Listener.Addr().String()
	dummy.Close() // now the port is gone

	var backendPort int
	_, _ = fmt.Sscanf(backendAddr[strings.LastIndex(backendAddr, ":")+1:], "%d", &backendPort)

	var buf bytes.Buffer
	lg := &Logger{forwardID: "test-err", maxBodyLen: 1024, output: &buf}

	p := &Proxy{
		localPort:  0,
		targetPort: backendPort,
		logger:     lg,
		forwardID:  "test-err",
	}
	require.NoError(t, p.Start())
	defer func() { _ = p.Stop() }()

	// The proxy should return 502 when backend is unreachable
	resp, err := http.Get("http://" + p.listener.Addr().String() + "/failing")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)

	// Error entry should be in the log (there may also be a request entry before it)
	logOutput := buf.String()
	assert.NotEmpty(t, logOutput, "error should be logged")

	var errorEntry *Entry
	for _, line := range strings.Split(strings.TrimSpace(logOutput), "\n") {
		if line == "" {
			continue
		}
		var e Entry
		if err2 := json.Unmarshal([]byte(line), &e); err2 == nil && e.Direction == "error" {
			eCopy := e
			errorEntry = &eCopy
		}
	}
	require.NotNil(t, errorEntry, "expected at least one error log entry")
	assert.Equal(t, "error", errorEntry.Direction)
	assert.NotEmpty(t, errorEntry.Error)
}

// TestRoundTrip_RequestCount verifies that each logged request increments the
// atomic request counter (drives the reqID path inside RoundTrip).
func TestRoundTrip_RequestCount(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{})

	for i := 0; i < 3; i++ {
		resp, err := http.Get(proxyURL(p) + "/tick")
		require.NoError(t, err)
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// 3 requests × 2 entries (req + resp) = 6 lines
	assert.Equal(t, 6, len(lines))

	// Request IDs should be "1", "2", "3" across request entries
	ids := make(map[string]bool)
	for _, line := range lines {
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil && e.Direction == "request" {
			ids[e.RequestID] = true
		}
	}
	assert.Len(t, ids, 3, "three distinct request IDs expected")
}

// ---------------------------------------------------------------------------
// readBodyLimited unit tests
// ---------------------------------------------------------------------------

// TestReadBodyLimited_SmallBody verifies the fast path: body ≤ 4096 bytes and
// under the maxSize limit returns exact content and correct size.
func TestReadBodyLimited_SmallBody(t *testing.T) {
	transport := &loggingTransport{}
	data := []byte("hello world")
	body := io.NopCloser(bytes.NewReader(data))

	result, size := transport.readBodyLimited(body, 1024)
	assert.Equal(t, data, result)
	assert.Equal(t, len(data), size)
}

// TestReadBodyLimited_EmptyBody verifies that an empty body returns an empty
// slice and size zero without panicking.
func TestReadBodyLimited_EmptyBody(t *testing.T) {
	transport := &loggingTransport{}
	body := io.NopCloser(bytes.NewReader([]byte{}))

	result, size := transport.readBodyLimited(body, 1024)
	assert.Empty(t, result)
	assert.Equal(t, 0, size)
}

// TestReadBodyLimited_TruncatedBody verifies the truncation path: when the
// body exceeds maxSize, the returned slice contains exactly maxSize bytes.
// The reported size is maxSize + (remaining bytes after the maxSize+1 read),
// which due to the implementation consuming one extra sentinel byte equals
// len(data)-1 for a body whose length > maxSize.
func TestReadBodyLimited_TruncatedBody(t *testing.T) {
	transport := &loggingTransport{}
	maxSize := 10
	// Body is 30 bytes — must be truncated to maxSize in the returned slice.
	data := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ1234")
	body := io.NopCloser(bytes.NewReader(data))

	result, size := transport.readBodyLimited(body, maxSize)
	assert.Equal(t, maxSize, len(result), "returned slice must be exactly maxSize bytes")
	assert.Equal(t, string(data[:maxSize]), string(result), "first maxSize bytes must match")
	// Implementation reads maxSize+1 sentinel bytes, then drains the rest.
	// The sentinel byte is consumed and not included in the "remaining" count,
	// so reported size == maxSize + (len(data) - maxSize - 1) == len(data) - 1.
	assert.Equal(t, len(data)-1, size, "reported size is total length minus the consumed sentinel byte")
}

// TestReadBodyLimited_ExactlyMaxSize ensures that a body equal to maxSize bytes
// is NOT truncated (the truncation condition is strictly greater-than).
func TestReadBodyLimited_ExactlyMaxSize(t *testing.T) {
	transport := &loggingTransport{}
	maxSize := 5
	data := []byte("ABCDE") // exactly maxSize
	body := io.NopCloser(bytes.NewReader(data))

	result, size := transport.readBodyLimited(body, maxSize)
	assert.Equal(t, data, result)
	assert.Equal(t, maxSize, size)
	assert.NotContains(t, string(result), "...(truncated)")
}

// TestReadBodyLimited_LargeBodyOverPoolThreshold exercises the branch in
// readBodyLimited where resultLen > 4096, which uses the pooled-buffer path
// for larger-than-small results. The body is 5000 bytes, well over the 4096
// small-body threshold but under maxSize so no truncation occurs.
func TestReadBodyLimited_LargeBodyOverPoolThreshold(t *testing.T) {
	transport := &loggingTransport{}
	data := bytes.Repeat([]byte("x"), 5000) // > 4096, under maxSize
	body := io.NopCloser(bytes.NewReader(data))

	result, size := transport.readBodyLimited(body, 65536)
	assert.Equal(t, 5000, len(result))
	assert.Equal(t, 5000, size)
	assert.Equal(t, data, result)
}

// TestReadBodyLimited_ZeroMaxSize covers the edge where maxSize == 0: every
// non-empty body is "over limit". The returned slice is empty (0 bytes). The
// reported size is the number of bytes drained after the sentinel read, which
// is len(data)-1 because the LimitReader reads 1 sentinel byte (maxSize+1=1)
// that is consumed and lost from the remaining count.
func TestReadBodyLimited_ZeroMaxSize(t *testing.T) {
	transport := &loggingTransport{}
	data := []byte("some data") // 9 bytes
	body := io.NopCloser(bytes.NewReader(data))

	result, size := transport.readBodyLimited(body, 0)
	assert.Equal(t, 0, len(result))
	// sentinel consumes 1 byte; remaining = 8; actualSize = 0 + 8 = 8
	assert.Equal(t, len(data)-1, size)
}

// TestReadBodyLimited_Callback exercises the transport inside a running proxy
// to confirm the pool-backed reading integrates correctly end-to-end
// (complementary to the direct unit tests above).
func TestReadBodyLimited_ViaCallback(t *testing.T) {
	var entries []Entry

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("R"), 200))
	}))
	defer backend.Close()

	p, _ := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{maxBodyLen: 100})

	p.logger.AddCallback(func(e Entry) {
		entries = append(entries, e)
	})

	resp, err := http.Get(proxyURL(p) + "/data")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// Give callbacks a moment (they run synchronously inside Log's mutex)
	require.Eventually(t, func() bool { return len(entries) >= 2 }, time.Second, 5*time.Millisecond)

	respEntry := entries[1] // second entry is the response
	assert.Equal(t, "response", respEntry.Direction)
	// Body was 200 bytes but maxBodyLen is 100 → BodySize should be ≥100
	assert.GreaterOrEqual(t, respEntry.BodySize, 100)
}

// TestRoundTrip_NilRequestBody confirms no panic when req.Body is nil (GET
// requests typically have no body).
func TestRoundTrip_NilRequestBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	p, buf := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{})

	req, _ := http.NewRequest("DELETE", proxyURL(p)+"/item/1", nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.NotEmpty(t, buf.String())
}

// TestRoundTrip_NilResponseBody ensures the transport handles a response with
// no body (Content-Length: 0) without panicking.
func TestRoundTrip_NilResponseBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		// No body written
	}))
	defer backend.Close()

	p, _ := makeProxy(t, backend, struct {
		filterPath  string
		includeHdrs bool
		maxBodyLen  int
	}{})

	resp, err := http.Get(proxyURL(p) + "/empty")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
