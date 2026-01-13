package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKlogWriter(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedLevel string
		expectedMsg   string
		description   string
		loggerLevel   Level
		loggerFormat  Format
		shouldLog     bool
	}{
		{
			name:          "info level log",
			input:         "I1124 12:34:56.789012   12345 portforward.go:123] Starting port forward\n",
			expectedLevel: "DEBUG",
			expectedMsg:   "Starting port forward",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     true,
			description:   "Info logs from k8s should be routed as DEBUG",
		},
		{
			name:          "warning level log",
			input:         "W1124 12:34:56.789012   12345 portforward.go:456] Connection unstable\n",
			expectedLevel: "WARN",
			expectedMsg:   "Connection unstable",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     true,
			description:   "Warning logs should be routed as WARN",
		},
		{
			name:          "error level log",
			input:         "E1124 12:34:56.789012   12345 portforward.go:789] Connection failed\n",
			expectedLevel: "ERROR",
			expectedMsg:   "Connection failed",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     true,
			description:   "Error logs should be routed as ERROR",
		},
		{
			name:          "fatal level log",
			input:         "F1124 12:34:56.789012   12345 portforward.go:999] Fatal error\n",
			expectedLevel: "ERROR",
			expectedMsg:   "Fatal error",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     true,
			description:   "Fatal logs should be routed as ERROR",
		},
		{
			name:          "multiline input",
			input:         "I1124 12:34:56.789012   12345 portforward.go:123] First message\nI1124 12:34:57.123456   12345 portforward.go:124] Second message\n",
			expectedLevel: "DEBUG",
			expectedMsg:   "First message",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     true,
			description:   "Should handle multiple log lines",
		},
		{
			name:          "log filtered by level",
			input:         "I1124 12:34:56.789012   12345 portforward.go:123] Debug message\n",
			expectedLevel: "DEBUG",
			expectedMsg:   "Debug message",
			loggerLevel:   LevelInfo, // Logger set to INFO, DEBUG should be filtered
			loggerFormat:  FormatText,
			shouldLog:     false,
			description:   "DEBUG logs should be filtered when logger level is INFO",
		},
		{
			name:          "unknown log format",
			input:         "X1124 12:34:56.789012   12345 portforward.go:123] Unknown format\n",
			expectedLevel: "DEBUG",
			expectedMsg:   "Unknown format",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     true,
			description:   "Unknown format should default to DEBUG",
		},
		{
			name:          "empty line",
			input:         "\n",
			expectedLevel: "",
			expectedMsg:   "",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     false,
			description:   "Empty lines should be ignored",
		},
		{
			name:          "partial line no newline",
			input:         "I1124 12:34:56.789012   12345 portforward.go:123] Partial",
			expectedLevel: "",
			expectedMsg:   "",
			loggerLevel:   LevelDebug,
			loggerFormat:  FormatText,
			shouldLog:     false,
			description:   "Partial lines without newline should be buffered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create output buffer
			var buf bytes.Buffer

			// Create logger with specified level and format
			logger := New(tt.loggerLevel, tt.loggerFormat, &buf)

			// Create klog writer
			klogWriter := NewKlogWriter(logger)

			// Write input
			n, err := klogWriter.Write([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, len(tt.input), n)

			// Check output
			output := buf.String()

			if !tt.shouldLog {
				assert.Empty(t, output, "Expected no log output")
				return
			}

			if tt.loggerFormat == FormatText {
				// Text format: [LEVEL] message
				assert.Contains(t, output, fmt.Sprintf("[%s]", tt.expectedLevel))
				assert.Contains(t, output, tt.expectedMsg)
				assert.Contains(t, output, "k8s-client") // Should include source field
			} else {
				// JSON format
				var entry logEntry
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) > 0 {
					err := json.Unmarshal([]byte(lines[0]), &entry)
					require.NoError(t, err)
					assert.Equal(t, tt.expectedLevel, entry.Level)
					assert.Equal(t, tt.expectedMsg, entry.Message)
					assert.Equal(t, "k8s-client", entry.Fields["source"])
				}
			}
		})
	}
}

func TestKlogWriterBuffering(t *testing.T) {
	tests := []struct {
		name        string
		description string
		writes      []string
		expectCount int
	}{
		{
			name: "single complete line",
			writes: []string{
				"I1124 12:34:56.789012   12345 portforward.go:123] Complete line\n",
			},
			expectCount: 1,
			description: "Single complete line should produce one log entry",
		},
		{
			name: "partial then complete",
			writes: []string{
				"I1124 12:34:56.789012   12345 portforward.go:123] Partial ",
				"line\n",
			},
			expectCount: 1,
			description: "Partial writes should be buffered and combined",
		},
		{
			name: "multiple complete lines in chunks",
			writes: []string{
				"I1124 12:34:56.789012   12345 portforward.go:123] First\n",
				"I1124 12:34:57.123456   12345 portforward.go:124] Second\n",
				"I1124 12:34:58.456789   12345 portforward.go:125] Third\n",
			},
			expectCount: 3,
			description: "Multiple complete lines should produce multiple log entries",
		},
		{
			name: "mixed partial and complete",
			writes: []string{
				"I1124 12:34:56.789012   12345 portforward.go:123] First\nI1124 12:34:57.123456   12345 port",
				"forward.go:124] Second\n",
			},
			expectCount: 2,
			description: "Mixed partial and complete lines should be handled correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(LevelDebug, FormatText, &buf)
			klogWriter := NewKlogWriter(logger)

			// Write all chunks
			for _, write := range tt.writes {
				_, err := klogWriter.Write([]byte(write))
				require.NoError(t, err)
			}

			// Count log entries (each line starts with [LEVEL])
			output := buf.String()
			count := strings.Count(output, "[DEBUG]") +
				strings.Count(output, "[INFO]") +
				strings.Count(output, "[WARN]") +
				strings.Count(output, "[ERROR]")

			assert.Equal(t, tt.expectCount, count, "Expected %d log entries, got %d", tt.expectCount, count)
		})
	}
}

func TestKlogWriterJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(LevelDebug, FormatJSON, &buf)
	klogWriter := NewKlogWriter(logger)

	// Write a k8s log line
	input := "I1124 12:34:56.789012   12345 portforward.go:123] Starting port forward\n"
	_, err := klogWriter.Write([]byte(input))
	require.NoError(t, err)

	// Parse JSON output
	var entry logEntry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	// Verify JSON structure
	assert.Equal(t, "DEBUG", entry.Level)
	assert.Equal(t, "Starting port forward", entry.Message)
	assert.NotEmpty(t, entry.Time)
	assert.Equal(t, "k8s-client", entry.Fields["source"])
}

func TestKlogWriterConcurrency(t *testing.T) {
	// Test that concurrent writes don't cause data races
	var buf bytes.Buffer
	logger := New(LevelDebug, FormatText, &buf)
	klogWriter := NewKlogWriter(logger)

	done := make(chan bool)
	numGoroutines := 10
	numWrites := 100

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numWrites; j++ {
				msg := fmt.Sprintf("I1124 12:34:56.789012   12345 test.go:123] Message from goroutine %d iteration %d\n", id, j)
				_, _ = klogWriter.Write([]byte(msg))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Just verify we didn't panic (data race detector would catch issues)
	assert.NotEmpty(t, buf.String())
}
