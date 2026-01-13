// Package logger provides structured logging with support for text and JSON
// output formats. It intercepts Kubernetes client-go logs and routes them
// through the structured logger.
//
// The package provides both instance-based and global logging:
//
//	// Instance-based logging
//	log := logger.New(logger.LevelInfo, logger.FormatJSON, os.Stderr)
//	log.Info("message", "key", "value")
//
//	// Global logging (after Init)
//	logger.Init(logger.LevelInfo, logger.FormatText, os.Stderr)
//	logger.Info("message", "key", "value")
//
// Log levels: DEBUG < INFO < WARN < ERROR
// Output formats: FormatText (human-readable), FormatJSON (structured)
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents the logging level.
// Higher levels include all lower levels (e.g., LevelInfo includes WARN and ERROR).
type Level int

const (
	// LevelDebug is for detailed troubleshooting information.
	LevelDebug Level = iota
	// LevelInfo is for general operational information.
	LevelInfo
	// LevelWarn is for unexpected but handled situations.
	LevelWarn
	// LevelError is for failures that require attention.
	LevelError
)

// Format represents the output format for log entries.
type Format int

const (
	// FormatText outputs human-readable log lines.
	FormatText Format = iota
	// FormatJSON outputs structured JSON log entries.
	FormatJSON
)

// Logger is a structured logger with configurable level and format.
// It is safe for concurrent use.
type Logger struct {
	output io.Writer
	level  Level
	format Format
	mu     sync.Mutex
}

// logEntry represents a single log entry for JSON output.
type logEntry struct {
	Fields  map[string]any `json:"fields,omitempty"`
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
}

// New creates a new Logger with the specified level, format, and output writer.
// If output is nil, os.Stderr is used.
func New(level Level, format Format, output io.Writer) *Logger {
	if output == nil {
		output = os.Stderr
	}
	return &Logger{
		level:  level,
		format: format,
		output: output,
	}
}

func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	levelStr := levelToString(level)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.format == FormatJSON {
		entry := logEntry{
			Time:    time.Now().Format(time.RFC3339),
			Level:   levelStr,
			Message: msg,
			Fields:  fields,
		}
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.output, string(data))
	} else {
		// Text format
		if len(fields) > 0 {
			fmt.Fprintf(l.output, "[%s] %s %v\n", levelStr, msg, fields)
		} else {
			fmt.Fprintf(l.output, "[%s] %s\n", levelStr, msg)
		}
	}
}

func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	f := make(map[string]interface{})
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, msg, f)
}

func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	f := make(map[string]interface{})
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	f := make(map[string]interface{})
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	f := make(map[string]interface{})
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}

func levelToString(level Level) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Global logger for backward compatibility
var globalLogger *Logger

func Init(level Level, format Format, output ...io.Writer) {
	var out io.Writer
	if len(output) > 0 && output[0] != nil {
		out = output[0]
	} else {
		out = os.Stderr
	}
	globalLogger = New(level, format, out)
}

func Debug(msg string, fields ...map[string]interface{}) {
	if globalLogger != nil {
		globalLogger.Debug(msg, fields...)
	}
}

func Info(msg string, fields ...map[string]interface{}) {
	if globalLogger != nil {
		globalLogger.Info(msg, fields...)
	}
}

func Warn(msg string, fields ...map[string]interface{}) {
	if globalLogger != nil {
		globalLogger.Warn(msg, fields...)
	}
}

func Error(msg string, fields ...map[string]interface{}) {
	if globalLogger != nil {
		globalLogger.Error(msg, fields...)
	}
}
