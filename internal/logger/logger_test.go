package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerTextFormat(t *testing.T) {
	tests := []struct {
		name           string
		level          Level
		logLevel       Level
		message        string
		fields         map[string]interface{}
		expectOutput   bool
		expectContains []string
	}{
		{
			name:           "info logged at info level",
			level:          LevelInfo,
			logLevel:       LevelInfo,
			message:        "test message",
			fields:         nil,
			expectOutput:   true,
			expectContains: []string{"[INFO]", "test message"},
		},
		{
			name:           "debug filtered at info level",
			level:          LevelInfo,
			logLevel:       LevelDebug,
			message:        "debug message",
			fields:         nil,
			expectOutput:   false,
			expectContains: []string{},
		},
		{
			name:           "error logged at info level",
			level:          LevelInfo,
			logLevel:       LevelError,
			message:        "error message",
			fields:         nil,
			expectOutput:   true,
			expectContains: []string{"[ERROR]", "error message"},
		},
		{
			name:     "info with fields",
			level:    LevelInfo,
			logLevel: LevelInfo,
			message:  "test message",
			fields: map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			},
			expectOutput:   true,
			expectContains: []string{"[INFO]", "test message", "key1", "value1"},
		},
		{
			name:           "warn logged at warn level",
			level:          LevelWarn,
			logLevel:       LevelWarn,
			message:        "warning message",
			fields:         nil,
			expectOutput:   true,
			expectContains: []string{"[WARN]", "warning message"},
		},
		{
			name:           "info filtered at warn level",
			level:          LevelWarn,
			logLevel:       LevelInfo,
			message:        "info message",
			fields:         nil,
			expectOutput:   false,
			expectContains: []string{},
		},
		{
			name:           "debug logged at debug level",
			level:          LevelDebug,
			logLevel:       LevelDebug,
			message:        "debug message",
			fields:         nil,
			expectOutput:   true,
			expectContains: []string{"[DEBUG]", "debug message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(tt.level, FormatText, buf)

			// Log at the specified level
			switch tt.logLevel {
			case LevelDebug:
				if tt.fields != nil {
					logger.Debug(tt.message, tt.fields)
				} else {
					logger.Debug(tt.message)
				}
			case LevelInfo:
				if tt.fields != nil {
					logger.Info(tt.message, tt.fields)
				} else {
					logger.Info(tt.message)
				}
			case LevelWarn:
				if tt.fields != nil {
					logger.Warn(tt.message, tt.fields)
				} else {
					logger.Warn(tt.message)
				}
			case LevelError:
				if tt.fields != nil {
					logger.Error(tt.message, tt.fields)
				} else {
					logger.Error(tt.message)
				}
			}

			output := buf.String()

			if tt.expectOutput {
				assert.NotEmpty(t, output, "Expected log output but got none")
				for _, expected := range tt.expectContains {
					assert.Contains(t, output, expected, "Expected output to contain: %s", expected)
				}
			} else {
				assert.Empty(t, output, "Expected no log output but got: %s", output)
			}
		})
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	tests := []struct {
		name         string
		level        Level
		logLevel     Level
		message      string
		fields       map[string]interface{}
		expectOutput bool
		expectLevel  string
	}{
		{
			name:         "info logged at info level",
			level:        LevelInfo,
			logLevel:     LevelInfo,
			message:      "test message",
			fields:       nil,
			expectOutput: true,
			expectLevel:  "INFO",
		},
		{
			name:         "debug filtered at info level",
			level:        LevelInfo,
			logLevel:     LevelDebug,
			message:      "debug message",
			fields:       nil,
			expectOutput: false,
			expectLevel:  "",
		},
		{
			name:         "error logged at debug level",
			level:        LevelDebug,
			logLevel:     LevelError,
			message:      "error message",
			fields:       nil,
			expectOutput: true,
			expectLevel:  "ERROR",
		},
		{
			name:     "info with fields",
			level:    LevelInfo,
			logLevel: LevelInfo,
			message:  "test message",
			fields: map[string]interface{}{
				"context": "production",
				"port":    8080,
				"retry":   3,
			},
			expectOutput: true,
			expectLevel:  "INFO",
		},
		{
			name:         "warn at warn level",
			level:        LevelWarn,
			logLevel:     LevelWarn,
			message:      "warning message",
			fields:       nil,
			expectOutput: true,
			expectLevel:  "WARN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(tt.level, FormatJSON, buf)

			// Log at the specified level
			switch tt.logLevel {
			case LevelDebug:
				if tt.fields != nil {
					logger.Debug(tt.message, tt.fields)
				} else {
					logger.Debug(tt.message)
				}
			case LevelInfo:
				if tt.fields != nil {
					logger.Info(tt.message, tt.fields)
				} else {
					logger.Info(tt.message)
				}
			case LevelWarn:
				if tt.fields != nil {
					logger.Warn(tt.message, tt.fields)
				} else {
					logger.Warn(tt.message)
				}
			case LevelError:
				if tt.fields != nil {
					logger.Error(tt.message, tt.fields)
				} else {
					logger.Error(tt.message)
				}
			}

			output := buf.String()

			if tt.expectOutput {
				assert.NotEmpty(t, output, "Expected log output but got none")

				// Parse JSON
				var entry logEntry
				err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry)
				require.NoError(t, err, "Failed to parse JSON output: %s", output)

				// Validate fields
				assert.Equal(t, tt.expectLevel, entry.Level)
				assert.Equal(t, tt.message, entry.Message)
				assert.NotEmpty(t, entry.Time, "Time field should not be empty")

				// Validate custom fields if provided
				if tt.fields != nil {
					require.NotNil(t, entry.Fields, "Expected fields in JSON output")
					for key, expectedValue := range tt.fields {
						actualValue, exists := entry.Fields[key]
						assert.True(t, exists, "Expected field %s not found in output", key)
						// JSON unmarshaling converts numbers to float64
						if floatVal, ok := expectedValue.(int); ok {
							assert.Equal(t, float64(floatVal), actualValue)
						} else {
							assert.Equal(t, expectedValue, actualValue)
						}
					}
				}
			} else {
				assert.Empty(t, output, "Expected no log output but got: %s", output)
			}
		})
	}
}

func TestGlobalLogger(t *testing.T) {
	tests := []struct {
		name           string
		initLevel      Level
		initFormat     Format
		logFunc        func(string, ...map[string]interface{})
		message        string
		expectContains string
	}{
		{
			name:           "global info logger text",
			initLevel:      LevelInfo,
			initFormat:     FormatText,
			logFunc:        Info,
			message:        "global info message",
			expectContains: "[INFO]",
		},
		{
			name:           "global error logger text",
			initLevel:      LevelInfo,
			initFormat:     FormatText,
			logFunc:        Error,
			message:        "global error message",
			expectContains: "[ERROR]",
		},
		{
			name:           "global warn logger json",
			initLevel:      LevelWarn,
			initFormat:     FormatJSON,
			logFunc:        Warn,
			message:        "global warn message",
			expectContains: `"level":"WARN"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stderr by replacing globalLogger's output
			buf := &bytes.Buffer{}
			Init(tt.initLevel, tt.initFormat)
			globalLogger.output = buf

			// Call the global log function
			tt.logFunc(tt.message)

			output := buf.String()
			assert.Contains(t, output, tt.expectContains)
			assert.Contains(t, output, tt.message)
		})
	}
}

func TestLogLevelsFiltering(t *testing.T) {
	tests := []struct {
		name          string
		loggerLevel   Level
		logAtLevels   []Level
		expectOutputs []bool
	}{
		{
			name:          "debug level logs everything",
			loggerLevel:   LevelDebug,
			logAtLevels:   []Level{LevelDebug, LevelInfo, LevelWarn, LevelError},
			expectOutputs: []bool{true, true, true, true},
		},
		{
			name:          "info level filters debug",
			loggerLevel:   LevelInfo,
			logAtLevels:   []Level{LevelDebug, LevelInfo, LevelWarn, LevelError},
			expectOutputs: []bool{false, true, true, true},
		},
		{
			name:          "warn level filters debug and info",
			loggerLevel:   LevelWarn,
			logAtLevels:   []Level{LevelDebug, LevelInfo, LevelWarn, LevelError},
			expectOutputs: []bool{false, false, true, true},
		},
		{
			name:          "error level only logs errors",
			loggerLevel:   LevelError,
			logAtLevels:   []Level{LevelDebug, LevelInfo, LevelWarn, LevelError},
			expectOutputs: []bool{false, false, false, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(tt.loggerLevel, FormatText, buf)

			for i, logLevel := range tt.logAtLevels {
				buf.Reset()

				switch logLevel {
				case LevelDebug:
					logger.Debug("test")
				case LevelInfo:
					logger.Info("test")
				case LevelWarn:
					logger.Warn("test")
				case LevelError:
					logger.Error("test")
				}

				hasOutput := buf.Len() > 0
				assert.Equal(t, tt.expectOutputs[i], hasOutput,
					"Level %v at logger level %v: expected output=%v, got=%v",
					logLevel, tt.loggerLevel, tt.expectOutputs[i], hasOutput)
			}
		})
	}
}

func TestLoggerNilOutput(t *testing.T) {
	// Test that logger defaults to os.Stderr when output is nil
	logger := New(LevelInfo, FormatText, nil)
	assert.NotNil(t, logger.output, "Logger output should not be nil")
}

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := levelToString(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONFieldTypes(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]interface{}
	}{
		{
			name: "string fields",
			fields: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "numeric fields",
			fields: map[string]interface{}{
				"port":    8080,
				"timeout": 30,
				"retry":   3,
			},
		},
		{
			name: "boolean fields",
			fields: map[string]interface{}{
				"enabled": true,
				"running": false,
			},
		},
		{
			name: "mixed types",
			fields: map[string]interface{}{
				"context":   "production",
				"port":      8080,
				"enabled":   true,
				"namespace": "default",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := New(LevelInfo, FormatJSON, buf)

			logger.Info("test message", tt.fields)

			var entry logEntry
			err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry)
			require.NoError(t, err)

			assert.Equal(t, len(tt.fields), len(entry.Fields),
				"Field count mismatch")

			for key := range tt.fields {
				_, exists := entry.Fields[key]
				assert.True(t, exists, "Field %s not found in JSON output", key)
			}
		})
	}
}
