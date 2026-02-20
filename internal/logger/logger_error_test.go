package logger

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// errorWriter is a writer that always returns an error
type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, e.err
}

func TestJSONMarshalErrorFallback(t *testing.T) {
	tests := []struct {
		fields         map[string]interface{}
		name           string
		message        string
		expectContains []string
		expectFallback bool
	}{
		{
			name:    "normal fields marshal successfully",
			message: "test message",
			fields: map[string]interface{}{
				"key": "value",
				"num": 123,
			},
			expectFallback: false,
			expectContains: []string{`"message":"test message"`, `"level":"INFO"`},
		},
		{
			name:    "channel field causes marshal error",
			message: "marshal error message",
			fields: map[string]interface{}{
				"bad_field": make(chan int),
			},
			expectFallback: true,
			expectContains: []string{"[INFO]", "marshal error message", "json marshal error"},
		},
		{
			name:    "nested unmarshalable field causes error",
			message: "nested error",
			fields: map[string]interface{}{
				"nested": map[string]interface{}{
					"channel": make(chan int),
				},
			},
			expectFallback: true,
			expectContains: []string{"[INFO]", "nested error", "json marshal error"},
		},
		{
			name:           "empty fields marshal successfully",
			message:        "no fields",
			fields:         nil,
			expectFallback: false,
			expectContains: []string{`"message":"no fields"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &strings.Builder{}
			logger := New(LevelInfo, FormatJSON, &testWriter{Builder: buf})

			logger.Info(tt.message, tt.fields)

			output := buf.String()
			assert.NotEmpty(t, output, "Expected log output but got none")

			if tt.expectFallback {
				// Should contain fallback text format indicators
				for _, expected := range tt.expectContains {
					assert.Contains(t, output, expected, "Expected fallback output to contain: %s", expected)
				}
				// Should NOT be valid JSON
				assert.False(t, strings.HasPrefix(output, "{"), "Fallback should not start with {")
			} else {
				// Should be valid JSON format
				for _, expected := range tt.expectContains {
					assert.Contains(t, output, expected, "Expected JSON output to contain: %s", expected)
				}
			}
		})
	}
}

func TestWriteErrorHandling(t *testing.T) {
	tests := []struct {
		writeError  error
		name        string
		format      Format
		expectPanic bool
	}{
		{
			name:        "JSON format write error",
			format:      FormatJSON,
			writeError:  errors.New("write failed"),
			expectPanic: false, // Should silently ignore write errors
		},
		{
			name:        "text format write error",
			format:      FormatText,
			writeError:  errors.New("disk full"),
			expectPanic: false, // Should silently ignore write errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a writer that always returns an error
			errWriter := &errorWriter{err: tt.writeError}
			logger := New(LevelInfo, tt.format, errWriter)

			// This should not panic, even though write fails
			assert.NotPanics(t, func() {
				logger.Info("test message", map[string]interface{}{"key": "value"})
			}, "Logger should not panic on write error")
		})
	}
}

func TestMarshalErrorWithDifferentLevels(t *testing.T) {
	// Test that marshal error fallback works for all log levels
	levels := []struct {
		logFunc  func(*Logger, string, map[string]interface{})
		levelStr string
		level    Level
	}{
		{func(l *Logger, m string, f map[string]interface{}) { l.Debug(m, f) }, "DEBUG", LevelDebug},
		{func(l *Logger, m string, f map[string]interface{}) { l.Info(m, f) }, "INFO", LevelInfo},
		{func(l *Logger, m string, f map[string]interface{}) { l.Warn(m, f) }, "WARN", LevelWarn},
		{func(l *Logger, m string, f map[string]interface{}) { l.Error(m, f) }, "ERROR", LevelError},
	}

	for _, lvl := range levels {
		t.Run(lvl.levelStr, func(t *testing.T) {
			buf := &strings.Builder{}
			logger := New(lvl.level, FormatJSON, &testWriter{Builder: buf})

			// Use unmarshalable field to trigger error
			lvl.logFunc(logger, "error test", map[string]interface{}{
				"bad": make(chan int),
			})

			output := buf.String()
			assert.Contains(t, output, "["+lvl.levelStr+"]", "Fallback should contain correct level")
			assert.Contains(t, output, "error test", "Fallback should contain message")
			assert.Contains(t, output, "json marshal error", "Fallback should indicate marshal error")
		})
	}
}

// testWriter wraps strings.Builder to implement io.Writer
type testWriter struct {
	*strings.Builder
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	return w.Builder.Write(p)
}
