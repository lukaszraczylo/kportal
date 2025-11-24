package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

type Format int

const (
	FormatText Format = iota
	FormatJSON
)

type Logger struct {
	level  Level
	format Format
	output io.Writer
}

type logEntry struct {
	Time    string                 `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

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
