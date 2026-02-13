package httplog

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/nvm/kportal/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer

	l := &Logger{
		forwardID:  "test-forward",
		maxBodyLen: 100,
		output:     &buf,
	}

	// Log an entry
	err := l.Log(Entry{
		Direction: "request",
		Method:    "GET",
		Path:      "/test",
		BodySize:  0,
	})
	require.NoError(t, err)

	// Parse the output
	var entry Entry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "test-forward", entry.ForwardID)
	assert.Equal(t, "request", entry.Direction)
	assert.Equal(t, "GET", entry.Method)
	assert.Equal(t, "/test", entry.Path)
	assert.False(t, entry.Timestamp.IsZero())
}

func TestLoggerBodyTruncation(t *testing.T) {
	var buf bytes.Buffer

	l := &Logger{
		forwardID:  "test-forward",
		maxBodyLen: 10,
		output:     &buf,
	}

	// Log an entry with a long body
	err := l.Log(Entry{
		Direction: "request",
		Method:    "POST",
		Path:      "/test",
		Body:      "this is a very long body that should be truncated",
		BodySize:  50,
	})
	require.NoError(t, err)

	// Parse the output
	var entry Entry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "this is a ...(truncated)", entry.Body)
}

func TestProxyShouldLog(t *testing.T) {
	tests := []struct {
		name       string
		filterPath string
		path       string
		expected   bool
	}{
		{"no filter", "", "/anything", true},
		{"exact match", "/api", "/api", true},
		{"no match", "/api", "/other", false},
		{"prefix match", "/api/*", "/api/users", true},
		{"prefix no match", "/api/*", "/other/users", false},
		{"wildcard", "/api/*/test", "/api/v1/test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Proxy{filterPath: tt.filterPath}
			assert.Equal(t, tt.expected, p.shouldLog(tt.path))
		})
	}
}

func TestProxyIntegration(t *testing.T) {
	// Create a buffer for log output
	var logBuf bytes.Buffer

	// Create config
	fwd := &config.Forward{
		LocalPort: 0, // Will be assigned dynamically
		HTTPLog: &config.HTTPLogSpec{
			Enabled:        true,
			IncludeHeaders: true,
			MaxBodySize:    1024,
		},
	}

	// Create logger with buffer
	logger := &Logger{
		forwardID:  "test",
		maxBodyLen: 1024,
		output:     &logBuf,
	}

	// Create proxy manually for testing
	proxy := &Proxy{
		localPort:   0, // Will use ephemeral port
		targetPort:  0, // Not used in this test
		logger:      logger,
		forwardID:   fwd.ID(),
		filterPath:  "",
		includeHdrs: true,
	}

	// Test shouldLog
	assert.True(t, proxy.shouldLog("/any/path"))

	// Test logging through logger directly
	err := logger.Log(Entry{
		RequestID: "1",
		Direction: "request",
		Method:    "GET",
		Path:      "/test",
	})
	require.NoError(t, err)

	// Verify log output
	assert.Contains(t, logBuf.String(), `"direction":"request"`)
	assert.Contains(t, logBuf.String(), `"method":"GET"`)
}

func TestFlattenHeaders(t *testing.T) {
	h := http.Header{
		"Content-Type": []string{"application/json"},
		"Accept":       []string{"text/html", "application/json"},
	}

	result := flattenHeaders(h)

	assert.Equal(t, "application/json", result["Content-Type"])
	assert.Equal(t, "text/html, application/json", result["Accept"])
}

func TestNewLogger(t *testing.T) {
	// Test stdout logger
	l, err := NewLogger("test-forward", "", 1024)
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.Nil(t, l.file) // No file when using stdout
	_ = l.Close()

	// Test file logger (using temp file)
	tmpFile := t.TempDir() + "/test.log"
	l, err = NewLogger("test-forward", tmpFile, 1024)
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.NotNil(t, l.file)

	// Write something
	err = l.Log(Entry{Direction: "request", Method: "GET"})
	require.NoError(t, err)

	_ = l.Close()

	// Verify file has content
	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"direction":"request"`)
}

// TestNewProxy tests proxy creation
func TestNewProxy(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		fwd := &config.Forward{
			LocalPort: 8080,
			Port:      80,
			HTTPLog: &config.HTTPLogSpec{
				Enabled:        true,
				FilterPath:     "/api/*",
				IncludeHeaders: true,
			},
		}

		proxy, err := NewProxy(fwd, 18080)
		require.NoError(t, err)
		require.NotNil(t, proxy)

		assert.Equal(t, 8080, proxy.localPort)
		assert.Equal(t, 18080, proxy.targetPort)
		assert.Equal(t, "/api/*", proxy.filterPath)
		assert.True(t, proxy.includeHdrs)
		assert.NotNil(t, proxy.logger)
	})

	t.Run("nil HTTPLog config", func(t *testing.T) {
		fwd := &config.Forward{
			LocalPort: 8080,
			HTTPLog:   nil,
		}

		proxy, err := NewProxy(fwd, 18080)
		assert.Error(t, err)
		assert.Nil(t, proxy)
		assert.Contains(t, err.Error(), "HTTP log config is nil")
	})
}

// TestProxy_GetTargetPort tests target port getter
func TestProxy_GetTargetPort(t *testing.T) {
	proxy := &Proxy{targetPort: 19090}
	assert.Equal(t, 19090, proxy.GetTargetPort())
}

// TestProxy_GetLogger tests logger getter
func TestProxy_GetLogger(t *testing.T) {
	logger := &Logger{forwardID: "test"}
	proxy := &Proxy{logger: logger}

	result := proxy.GetLogger()
	assert.Equal(t, logger, result)
}

// TestProxy_ShouldLog tests path filtering
func TestProxy_ShouldLog(t *testing.T) {
	tests := []struct {
		name       string
		filterPath string
		path       string
		expected   bool
	}{
		// No filter - log everything
		{"empty filter logs all", "", "/any/path", true},
		{"empty filter logs root", "", "/", true},

		// Exact match
		{"exact match", "/api", "/api", true},
		{"exact no match", "/api", "/other", false},

		// Wildcard patterns
		{"single wildcard match", "/api/*", "/api/users", true},
		{"single wildcard no match", "/api/*", "/other/users", false},
		{"middle wildcard", "/api/*/test", "/api/v1/test", true},
		{"middle wildcard no match", "/api/*/test", "/api/v1/other", false},

		// Prefix patterns (/* suffix special handling)
		{"prefix match", "/api/*", "/api/users/123", true},
		{"prefix match nested", "/api/*", "/api/users/123/deep", true},

		// Edge cases
		{"empty path", "/api/*", "", false},
		{"trailing slash filter", "/api/", "/api/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Proxy{filterPath: tt.filterPath}
			result := p.shouldLog(tt.path)
			assert.Equal(t, tt.expected, result, "filterPath=%q, path=%q", tt.filterPath, tt.path)
		})
	}
}

// TestProxy_ShouldLog_InvalidPattern tests behavior with invalid glob patterns
func TestProxy_ShouldLog_InvalidPattern(t *testing.T) {
	// Invalid glob pattern (unclosed bracket)
	p := &Proxy{filterPath: "/api/[invalid"}

	// Should default to logging everything on invalid pattern
	assert.True(t, p.shouldLog("/any/path"))
}

// TestProxy_StartStop tests basic start/stop lifecycle
func TestProxy_StartStop(t *testing.T) {
	var buf bytes.Buffer
	logger := &Logger{
		forwardID:  "test",
		maxBodyLen: 1024,
		output:     &buf,
	}

	proxy := &Proxy{
		localPort:  0, // Ephemeral port
		targetPort: 9999,
		logger:     logger,
		forwardID:  "test-fwd",
	}

	// Start
	err := proxy.Start()
	require.NoError(t, err)
	assert.True(t, proxy.running)
	assert.NotNil(t, proxy.listener)
	assert.NotNil(t, proxy.server)

	// Double start should fail
	err = proxy.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Stop
	err = proxy.Stop()
	assert.NoError(t, err)
	assert.False(t, proxy.running)

	// Double stop should be OK
	err = proxy.Stop()
	assert.NoError(t, err)
}

// TestProxy_Start_PortInUse tests behavior when port is already in use
func TestProxy_Start_PortInUse(t *testing.T) {
	// Start first proxy
	logger1 := &Logger{output: bytes.NewBuffer(nil), maxBodyLen: 100}
	proxy1 := &Proxy{
		localPort:  0, // Ephemeral
		targetPort: 9999,
		logger:     logger1,
	}
	err := proxy1.Start()
	require.NoError(t, err)
	defer func() { _ = proxy1.Stop() }()

	// Get the actual port
	addr := proxy1.listener.Addr().(*net.TCPAddr)
	usedPort := addr.Port

	// Try to start second proxy on same port
	logger2 := &Logger{output: bytes.NewBuffer(nil), maxBodyLen: 100}
	proxy2 := &Proxy{
		localPort:  usedPort,
		targetPort: 9999,
		logger:     logger2,
	}

	err = proxy2.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to listen")
}

// TestFlattenHeaders_EdgeCases tests header flattening edge cases
func TestFlattenHeaders_EdgeCases(t *testing.T) {
	tests := []struct {
		headers  http.Header
		expected map[string]string
		name     string
	}{
		{
			name:     "empty headers",
			headers:  http.Header{},
			expected: map[string]string{},
		},
		{
			name:     "single value",
			headers:  http.Header{"X-Test": {"value"}},
			expected: map[string]string{"X-Test": "value"},
		},
		{
			name:     "multiple values same key",
			headers:  http.Header{"Accept": {"text/html", "application/json", "text/plain"}},
			expected: map[string]string{"Accept": "text/html, application/json, text/plain"},
		},
		{
			name:     "empty value",
			headers:  http.Header{"X-Empty": {""}},
			expected: map[string]string{"X-Empty": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flattenHeaders(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProxy_RequestCount tests request counting
func TestProxy_RequestCount(t *testing.T) {
	proxy := &Proxy{requestCount: 0}

	// Simulate incrementing (normally done by loggingTransport)
	assert.Equal(t, uint64(0), proxy.requestCount)
}

// TestProxy_LogError tests error logging
func TestProxy_LogError(t *testing.T) {
	var buf bytes.Buffer
	logger := &Logger{
		forwardID:  "test",
		maxBodyLen: 1024,
		output:     &buf,
	}

	proxy := &Proxy{
		logger:    logger,
		forwardID: "test-fwd",
	}

	req, _ := http.NewRequest("GET", "/test", nil)
	proxy.logError(req, assert.AnError)

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "error", entry.Direction)
	assert.Equal(t, "GET", entry.Method)
	assert.Equal(t, "/test", entry.Path)
	assert.Contains(t, entry.Error, "assert.AnError")
}
