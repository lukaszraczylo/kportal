package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogrAdapter_Info(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		keysAndValues  []interface{}
		expectContains []string
		loggerLevel    Level
		logrLevel      int
		expectOutput   bool
	}{
		{
			name:           "info log v0 with debug logger",
			loggerLevel:    LevelDebug,
			logrLevel:      0,
			message:        "Connection established",
			keysAndValues:  []interface{}{"pod", "my-app-123", "port", 8080},
			expectOutput:   true,
			expectContains: []string{"[INFO]", "Connection established", "pod", "my-app-123"},
		},
		{
			name:           "info log v0 with info logger",
			loggerLevel:    LevelInfo,
			logrLevel:      0,
			message:        "Port forward ready",
			keysAndValues:  []interface{}{},
			expectOutput:   true,
			expectContains: []string{"[INFO]", "Port forward ready"},
		},
		{
			name:           "info log v0 silenced with warn logger",
			loggerLevel:    LevelWarn,
			logrLevel:      0,
			message:        "This should not appear",
			keysAndValues:  []interface{}{},
			expectOutput:   false,
			expectContains: []string{},
		},
		{
			name:           "debug log v1 with debug logger",
			loggerLevel:    LevelDebug,
			logrLevel:      1,
			message:        "Detailed connection info",
			keysAndValues:  []interface{}{"details", "some-value"},
			expectOutput:   true,
			expectContains: []string{"[DEBUG]", "Detailed connection info", "details"},
		},
		{
			name:           "debug log v1 silenced with info logger",
			loggerLevel:    LevelInfo,
			logrLevel:      1,
			message:        "This debug should not appear",
			keysAndValues:  []interface{}{},
			expectOutput:   false,
			expectContains: []string{},
		},
		{
			name:           "info with odd number of kvs (incomplete pair)",
			loggerLevel:    LevelInfo,
			logrLevel:      0,
			message:        "Message with incomplete kv",
			keysAndValues:  []interface{}{"key1", "value1", "key2"}, // key2 has no value
			expectOutput:   true,
			expectContains: []string{"[INFO]", "Message with incomplete kv", "key1", "value1"},
		},
		{
			name:           "info with source field added automatically",
			loggerLevel:    LevelInfo,
			logrLevel:      0,
			message:        "Test source field",
			keysAndValues:  []interface{}{},
			expectOutput:   true,
			expectContains: []string{"[INFO]", "Test source field", "source:k8s-client"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(tt.loggerLevel, FormatText, buf)
			sink := NewLogrAdapter(logger)
			logrLogger := logr.New(sink)

			logrLogger.V(tt.logrLevel).Info(tt.message, tt.keysAndValues...)

			output := buf.String()
			if tt.expectOutput {
				for _, expected := range tt.expectContains {
					assert.Contains(t, output, expected, "Output should contain: %s", expected)
				}
			} else {
				assert.Empty(t, output, "No output expected for this log level")
			}
		})
	}
}

func TestLogrAdapter_Error(t *testing.T) {
	tests := []struct {
		err            error
		name           string
		message        string
		keysAndValues  []interface{}
		expectContains []string
		loggerLevel    Level
		expectOutput   bool
	}{
		{
			name:           "error with error object",
			loggerLevel:    LevelError,
			err:            errors.New("connection failed"),
			message:        "Port forward failed",
			keysAndValues:  []interface{}{"pod", "my-app-123"},
			expectOutput:   true,
			expectContains: []string{"[ERROR]", "Port forward failed", "connection failed", "pod", "my-app-123"},
		},
		{
			name:           "error without error object",
			loggerLevel:    LevelError,
			err:            nil,
			message:        "Generic error message",
			keysAndValues:  []interface{}{},
			expectOutput:   true,
			expectContains: []string{"[ERROR]", "Generic error message"},
		},
		{
			name:           "error silenced with level above error",
			loggerLevel:    LevelError + 1,
			err:            errors.New("should not appear"),
			message:        "This error should not appear",
			keysAndValues:  []interface{}{},
			expectOutput:   false,
			expectContains: []string{},
		},
		{
			name:           "error with multiple kvs",
			loggerLevel:    LevelError,
			err:            errors.New("sandbox not found"),
			message:        "Unhandled Error",
			keysAndValues:  []interface{}{"pod", "test-pod", "uid", "abc123", "port", 8080},
			expectOutput:   true,
			expectContains: []string{"[ERROR]", "Unhandled Error", "sandbox not found", "pod", "test-pod", "uid", "abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(tt.loggerLevel, FormatText, buf)
			sink := NewLogrAdapter(logger)
			logrLogger := logr.New(sink)

			logrLogger.Error(tt.err, tt.message, tt.keysAndValues...)

			output := buf.String()
			if tt.expectOutput {
				for _, expected := range tt.expectContains {
					assert.Contains(t, output, expected, "Output should contain: %s", expected)
				}
			} else {
				assert.Empty(t, output, "No output expected for this log level")
			}
		})
	}
}

func TestLogrAdapter_WithName(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		expectContains string
		loggerNames    []string
	}{
		{
			name:           "single logger name",
			loggerNames:    []string{"portforward"},
			message:        "Test message",
			expectContains: "logger:portforward",
		},
		{
			name:           "nested logger names",
			loggerNames:    []string{"controller", "worker", "healthcheck"},
			message:        "Nested message",
			expectContains: "logger:controller.worker.healthcheck",
		},
		{
			name:           "no logger name",
			loggerNames:    []string{},
			message:        "No name message",
			expectContains: "source:k8s-client", // Should still have source but no logger field
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(LevelInfo, FormatText, buf)
			sink := NewLogrAdapter(logger)
			logrLogger := logr.New(sink)

			// Apply WithName calls
			for _, name := range tt.loggerNames {
				logrLogger = logrLogger.WithName(name)
			}

			logrLogger.Info(tt.message)

			output := buf.String()
			assert.Contains(t, output, tt.expectContains)
		})
	}
}

func TestLogrAdapter_Enabled(t *testing.T) {
	tests := []struct {
		name          string
		loggerLevel   Level
		logrLevel     int
		expectEnabled bool
	}{
		{
			name:          "v0 enabled with debug logger",
			loggerLevel:   LevelDebug,
			logrLevel:     0,
			expectEnabled: true,
		},
		{
			name:          "v0 enabled with info logger",
			loggerLevel:   LevelInfo,
			logrLevel:     0,
			expectEnabled: true,
		},
		{
			name:          "v0 disabled with warn logger",
			loggerLevel:   LevelWarn,
			logrLevel:     0,
			expectEnabled: false,
		},
		{
			name:          "v1 enabled with debug logger",
			loggerLevel:   LevelDebug,
			logrLevel:     1,
			expectEnabled: true,
		},
		{
			name:          "v1 disabled with info logger",
			loggerLevel:   LevelInfo,
			logrLevel:     1,
			expectEnabled: false,
		},
		{
			name:          "v2 enabled with debug logger",
			loggerLevel:   LevelDebug,
			logrLevel:     2,
			expectEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.loggerLevel, FormatText, &bytes.Buffer{})
			sink := NewLogrAdapter(logger)

			enabled := sink.Enabled(tt.logrLevel)
			assert.Equal(t, tt.expectEnabled, enabled)
		})
	}
}

func TestLogrAdapter_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(LevelInfo, FormatJSON, buf)
	sink := NewLogrAdapter(logger)
	logrLogger := logr.New(sink).WithName("test-component")

	logrLogger.Info("Test JSON message", "key1", "value1", "key2", 123)

	// Parse JSON output
	var entry logEntry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry.Level)
	assert.Equal(t, "Test JSON message", entry.Message)
	assert.Equal(t, "k8s-client", entry.Fields["source"])
	assert.Equal(t, "test-component", entry.Fields["logger"])
	assert.Equal(t, "value1", entry.Fields["key1"])
	assert.Equal(t, float64(123), entry.Fields["key2"]) // JSON numbers decode as float64
}

func TestLogrAdapter_ConcurrentWrites(t *testing.T) {
	// Note: bytes.Buffer is not thread-safe for writes, so this test verifies
	// that our LogrAdapter doesn't panic under concurrent load, but we don't
	// verify exact output (since logger uses fmt.Fprintf which is also not thread-safe)
	buf := &bytes.Buffer{}
	logger := New(LevelDebug, FormatText, buf)
	sink := NewLogrAdapter(logger)
	logrLogger := logr.New(sink)

	// Spawn multiple goroutines writing concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				logrLogger.Info("Concurrent message", "goroutine", id, "iteration", j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	output := buf.String()

	// Verify we got substantial output (not checking exact count due to buffer race)
	// The main goal is to ensure no panics occur during concurrent writes
	assert.NotEmpty(t, output, "Should have some log output")
	assert.Contains(t, output, "Concurrent message")
}

func TestLogrAdapter_RealWorldKlogError(t *testing.T) {
	// Simulate the exact error message from the screenshot
	buf := &bytes.Buffer{}
	logger := New(LevelError, FormatText, buf)
	sink := NewLogrAdapter(logger)
	logrLogger := logr.New(sink).WithName("UnhandledError")

	err := errors.New("an error occurred forwarding 8401 -> 8401: error forwarding port 8401 to pod 4e1e861c28e3b25a88b082e79788169b5d8a7a117904b7bb8c7cd59285cf1d308, uid : failed to find sandbox '4e1e861c28e3b25a88b082e79788169b5d8a7a117904b7bb8c7cd59285cf1d308' in store: not found")
	logrLogger.Error(err, "Unhandled Error")

	output := buf.String()
	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "Unhandled Error")
	assert.Contains(t, output, "failed to find sandbox")
	assert.Contains(t, output, "logger:UnhandledError")
}

func TestLogrAdapter_SilenceMode(t *testing.T) {
	// Test that logs are completely silenced when logger level is above error
	buf := &bytes.Buffer{}
	logger := New(LevelError+1, FormatText, buf)
	sink := NewLogrAdapter(logger)
	logrLogger := logr.New(sink)

	// Try all log levels
	logrLogger.V(0).Info("Info message should not appear")
	logrLogger.V(1).Info("Debug message should not appear")
	logrLogger.Error(errors.New("error object"), "Error message should not appear")

	output := buf.String()
	assert.Empty(t, output, "All logs should be silenced")
}
