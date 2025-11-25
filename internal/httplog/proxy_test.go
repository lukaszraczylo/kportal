package httplog

import (
	"bytes"
	"encoding/json"
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
	l.Close()

	// Test file logger (using temp file)
	tmpFile := t.TempDir() + "/test.log"
	l, err = NewLogger("test-forward", tmpFile, 1024)
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.NotNil(t, l.file)

	// Write something
	err = l.Log(Entry{Direction: "request", Method: "GET"})
	require.NoError(t, err)

	l.Close()

	// Verify file has content
	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"direction":"request"`)
}
