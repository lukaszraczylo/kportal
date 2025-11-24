package logger

import (
	"github.com/go-logr/logr"
)

// LogrAdapter implements the logr.LogSink interface to route klog v2 logs
// through our structured logger. This captures ALL klog output including
// error logs, structured logs, and named logger output.
type LogrAdapter struct {
	logger *Logger
	name   string
	level  int
}

// NewLogrAdapter creates a new logr.LogSink that routes all klog v2 logs
// through our structured logger.
func NewLogrAdapter(logger *Logger) logr.LogSink {
	return &LogrAdapter{
		logger: logger,
		name:   "",
		level:  0,
	}
}

// Init initializes the logger with runtime info (not used in our implementation).
func (l *LogrAdapter) Init(info logr.RuntimeInfo) {
	// No-op: we don't need runtime info
}

// Enabled tests whether this LogSink is enabled at the specified V-level.
// We route all logs through our logger's level filtering.
func (l *LogrAdapter) Enabled(level int) bool {
	// Map logr V-levels to our levels:
	// V(0) = Info level (always enabled if logger level <= Info)
	// V(1+) = Debug level (enabled if logger level <= Debug)
	if level == 0 {
		return l.logger.level <= LevelInfo
	}
	return l.logger.level <= LevelDebug
}

// Info logs a non-error message with the given key/value pairs.
func (l *LogrAdapter) Info(level int, msg string, keysAndValues ...interface{}) {
	fields := l.kvToMap(keysAndValues)
	if l.name != "" {
		fields["logger"] = l.name
	}

	// Map logr V-levels to our levels:
	// V(0) = Info, V(1+) = Debug
	if level == 0 {
		l.logger.Info(msg, fields)
	} else {
		l.logger.Debug(msg, fields)
	}
}

// Error logs an error message with the given key/value pairs.
func (l *LogrAdapter) Error(err error, msg string, keysAndValues ...interface{}) {
	fields := l.kvToMap(keysAndValues)
	if l.name != "" {
		fields["logger"] = l.name
	}
	if err != nil {
		fields["error"] = err.Error()
	}

	l.logger.Error(msg, fields)
}

// WithValues returns a new LogSink with additional key/value pairs.
func (l *LogrAdapter) WithValues(keysAndValues ...interface{}) logr.LogSink {
	// For simplicity, we don't implement value accumulation
	// Each log call receives all its keysAndValues directly
	return l
}

// WithName returns a new LogSink with the specified name appended.
func (l *LogrAdapter) WithName(name string) logr.LogSink {
	newLogger := *l
	if l.name == "" {
		newLogger.name = name
	} else {
		newLogger.name = l.name + "." + name
	}
	return &newLogger
}

// kvToMap converts a slice of alternating keys and values to a map.
func (l *LogrAdapter) kvToMap(keysAndValues []interface{}) map[string]interface{} {
	fields := make(map[string]interface{})
	fields["source"] = "k8s-client"

	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key, ok := keysAndValues[i].(string)
			if ok {
				fields[key] = keysAndValues[i+1]
			}
		}
	}

	return fields
}
