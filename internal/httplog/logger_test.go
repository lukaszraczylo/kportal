package httplog

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLogger_OutputModes tests different output configurations
func TestNewLogger_OutputModes(t *testing.T) {
	t.Run("empty logFile uses io.Discard", func(t *testing.T) {
		l, err := NewLogger("test-forward", "", 1024)
		require.NoError(t, err)
		defer l.Close()

		assert.Nil(t, l.file)
		assert.Equal(t, io.Discard, l.output)
		assert.Equal(t, "test-forward", l.forwardID)
		assert.Equal(t, 1024, l.maxBodyLen)
	})

	t.Run("file logger creates file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "http.log")

		l, err := NewLogger("test-forward", logFile, 2048)
		require.NoError(t, err)
		defer l.Close()

		assert.NotNil(t, l.file)
		assert.NotEqual(t, io.Discard, l.output)
		assert.Equal(t, 2048, l.maxBodyLen)

		// File should exist
		_, err = os.Stat(logFile)
		assert.NoError(t, err)
	})

	t.Run("file logger appends to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "http.log")

		// Create file with existing content
		err := os.WriteFile(logFile, []byte("existing\n"), 0600)
		require.NoError(t, err)

		l, err := NewLogger("test-forward", logFile, 1024)
		require.NoError(t, err)

		err = l.Log(Entry{Direction: "request"})
		require.NoError(t, err)
		l.Close()

		// File should have both contents
		data, _ := os.ReadFile(logFile)
		assert.True(t, strings.HasPrefix(string(data), "existing\n"))
		assert.Contains(t, string(data), "direction")
	})

	t.Run("invalid path returns error", func(t *testing.T) {
		_, err := NewLogger("test", "/nonexistent/path/file.log", 1024)
		assert.Error(t, err)
	})
}

// TestLogger_Log tests basic logging functionality
func TestLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{
		forwardID:  "fwd-123",
		maxBodyLen: 100,
		output:     &buf,
	}

	err := l.Log(Entry{
		Direction: "request",
		RequestID: "req-1",
		Method:    "POST",
		Path:      "/api/users",
		BodySize:  42,
		Body:      `{"name":"test"}`,
	})
	require.NoError(t, err)

	// Parse output
	var entry Entry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "fwd-123", entry.ForwardID)
	assert.Equal(t, "request", entry.Direction)
	assert.Equal(t, "req-1", entry.RequestID)
	assert.Equal(t, "POST", entry.Method)
	assert.Equal(t, "/api/users", entry.Path)
	assert.Equal(t, 42, entry.BodySize)
	assert.Equal(t, `{"name":"test"}`, entry.Body)
	assert.False(t, entry.Timestamp.IsZero())
}

// TestLogger_Log_Response tests response logging
func TestLogger_Log_Response(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{
		forwardID:  "fwd-123",
		maxBodyLen: 1000,
		output:     &buf,
	}

	err := l.Log(Entry{
		Direction:  "response",
		RequestID:  "req-1",
		Method:     "GET",
		Path:       "/api/status",
		StatusCode: 200,
		LatencyMs:  125,
		Headers:    map[string]string{"Content-Type": "application/json"},
	})
	require.NoError(t, err)

	var entry Entry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "response", entry.Direction)
	assert.Equal(t, 200, entry.StatusCode)
	assert.Equal(t, int64(125), entry.LatencyMs)
	assert.Equal(t, "application/json", entry.Headers["Content-Type"])
}

// TestLogger_Log_Error tests error logging
func TestLogger_Log_Error(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{
		forwardID:  "fwd-123",
		maxBodyLen: 100,
		output:     &buf,
	}

	err := l.Log(Entry{
		Direction: "error",
		RequestID: "req-1",
		Method:    "GET",
		Path:      "/api/fail",
		Error:     "connection refused",
	})
	require.NoError(t, err)

	var entry Entry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "error", entry.Direction)
	assert.Equal(t, "connection refused", entry.Error)
}

// TestLogger_BodyTruncation tests body size limiting
func TestLogger_BodyTruncation(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		maxBodyLen  int
		expectTrunc bool
	}{
		{name: "body under limit", maxBodyLen: 100, body: "short", expectTrunc: false},
		{name: "body at limit", maxBodyLen: 5, body: "exact", expectTrunc: false},
		{name: "body over limit", maxBodyLen: 5, body: "this is too long", expectTrunc: true},
		{name: "empty body", maxBodyLen: 100, body: "", expectTrunc: false},
		{name: "zero max", maxBodyLen: 0, body: "any", expectTrunc: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := &Logger{
				forwardID:  "test",
				maxBodyLen: tt.maxBodyLen,
				output:     &buf,
			}

			_ = l.Log(Entry{Body: tt.body})

			var entry Entry
			_ = json.Unmarshal(buf.Bytes(), &entry)

			if tt.expectTrunc {
				assert.Contains(t, entry.Body, "...(truncated)")
			} else {
				assert.NotContains(t, entry.Body, "truncated")
			}
		})
	}
}

// TestLogger_Callbacks tests callback registration and invocation
func TestLogger_Callbacks(t *testing.T) {
	l := &Logger{
		forwardID:  "test",
		maxBodyLen: 100,
		output:     io.Discard,
	}

	var received []Entry
	var mu sync.Mutex

	// Add callback
	l.AddCallback(func(entry Entry) {
		mu.Lock()
		received = append(received, entry)
		mu.Unlock()
	})

	// Log entries
	_ = l.Log(Entry{Direction: "request", Path: "/api/1"})
	_ = l.Log(Entry{Direction: "response", Path: "/api/1"})
	_ = l.Log(Entry{Direction: "request", Path: "/api/2"})

	mu.Lock()
	assert.Len(t, received, 3)
	assert.Equal(t, "/api/1", received[0].Path)
	assert.Equal(t, "response", received[1].Direction)
	mu.Unlock()
}

// TestLogger_MultipleCallbacks tests multiple callbacks
func TestLogger_MultipleCallbacks(t *testing.T) {
	l := &Logger{
		forwardID:  "test",
		maxBodyLen: 100,
		output:     io.Discard,
	}

	count1 := 0
	count2 := 0

	l.AddCallback(func(entry Entry) { count1++ })
	l.AddCallback(func(entry Entry) { count2++ })

	_ = l.Log(Entry{})

	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}

// TestLogger_ClearCallbacks tests callback clearing
func TestLogger_ClearCallbacks(t *testing.T) {
	l := &Logger{
		forwardID:  "test",
		maxBodyLen: 100,
		output:     io.Discard,
	}

	count := 0
	l.AddCallback(func(entry Entry) { count++ })

	_ = l.Log(Entry{})
	assert.Equal(t, 1, count)

	l.ClearCallbacks()

	_ = l.Log(Entry{})
	assert.Equal(t, 1, count) // Still 1 - callback was cleared
}

// TestLogger_GetMaxBodyLen tests the getter
func TestLogger_GetMaxBodyLen(t *testing.T) {
	l := &Logger{maxBodyLen: 4096}
	assert.Equal(t, 4096, l.GetMaxBodyLen())
}

// TestLogger_Close tests closing
func TestLogger_Close(t *testing.T) {
	t.Run("close with no file", func(t *testing.T) {
		l := &Logger{output: io.Discard}
		err := l.Close()
		assert.NoError(t, err)
	})

	t.Run("close with file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.log")
		l, err := NewLogger("test", tmpFile, 100)
		require.NoError(t, err)

		err = l.Close()
		assert.NoError(t, err)

		// File should be closed (writing should fail or create new handle)
		assert.NotNil(t, l.file) // reference still exists
	})
}

// TestLogger_Concurrent tests thread safety
func TestLogger_Concurrent(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{
		forwardID:  "test",
		maxBodyLen: 100,
		output:     &buf,
	}

	// Add callback that accesses shared state
	var callbackCount int
	var mu sync.Mutex
	l.AddCallback(func(entry Entry) {
		mu.Lock()
		callbackCount++
		mu.Unlock()
	})

	// Concurrent writes
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = l.Log(Entry{
				Direction: "request",
				Path:      "/api/" + string(rune('a'+n%26)),
			})
		}(i)
	}
	wg.Wait()

	mu.Lock()
	assert.Equal(t, 100, callbackCount)
	mu.Unlock()
}

// TestEntry_Structure tests the Entry struct
func TestEntry_Structure(t *testing.T) {
	now := time.Now()
	entry := Entry{
		Timestamp:  now,
		ForwardID:  "fwd-1",
		RequestID:  "req-1",
		Direction:  "request",
		Method:     "DELETE",
		Path:       "/api/items/123",
		StatusCode: 204,
		Headers:    map[string]string{"X-Custom": "value"},
		BodySize:   0,
		Body:       "",
		LatencyMs:  50,
		Error:      "",
	}

	// Verify all fields
	assert.Equal(t, now, entry.Timestamp)
	assert.Equal(t, "fwd-1", entry.ForwardID)
	assert.Equal(t, "req-1", entry.RequestID)
	assert.Equal(t, "request", entry.Direction)
	assert.Equal(t, "DELETE", entry.Method)
	assert.Equal(t, "/api/items/123", entry.Path)
	assert.Equal(t, 204, entry.StatusCode)
	assert.Equal(t, "value", entry.Headers["X-Custom"])
	assert.Equal(t, 0, entry.BodySize)
	assert.Empty(t, entry.Body)
	assert.Equal(t, int64(50), entry.LatencyMs)
	assert.Empty(t, entry.Error)
}

// TestEntry_JSONMarshaling tests JSON serialization
func TestEntry_JSONMarshaling(t *testing.T) {
	entry := Entry{
		Direction:  "response",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		LatencyMs:  100,
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var parsed Entry
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, entry.Direction, parsed.Direction)
	assert.Equal(t, entry.StatusCode, parsed.StatusCode)
}
