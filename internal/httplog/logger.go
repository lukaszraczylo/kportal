// Package httplog provides HTTP request/response logging for port forwards.
// It captures HTTP traffic passing through the forward proxy and stores
// entries for viewing in the UI.
//
// The logger supports:
//   - Request and response capture with headers and bodies
//   - Configurable body size limits to prevent memory issues
//   - Callback-based notifications for real-time log viewing
//   - Thread-safe operation for concurrent forwards
//
// Bodies are truncated if they exceed the configured maximum size
// (default: 1MB) and marked as truncated in the log entry.
package httplog

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Entry represents a single HTTP log entry
type Entry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Headers    map[string]string `json:"headers,omitempty"`
	ForwardID  string            `json:"forward_id"`
	RequestID  string            `json:"request_id"`
	Direction  string            `json:"direction"`
	Method     string            `json:"method,omitempty"`
	Path       string            `json:"path,omitempty"`
	Body       string            `json:"body,omitempty"`
	Error      string            `json:"error,omitempty"`
	StatusCode int               `json:"status_code,omitempty"`
	BodySize   int               `json:"body_size"`
	LatencyMs  int64             `json:"latency_ms,omitempty"`
}

// LogCallback is a function that receives log entries
type LogCallback func(entry Entry)

// Logger writes HTTP log entries to an output stream
type Logger struct {
	output     io.Writer
	file       *os.File
	forwardID  string
	callbacks  []LogCallback
	maxBodyLen int
	mu         sync.Mutex
}

// NewLogger creates a new HTTP logger
// If logFile is empty, logs only go to registered callbacks (no file output)
// This prevents stdout corruption when running in TUI mode
func NewLogger(forwardID, logFile string, maxBodyLen int) (*Logger, error) {
	l := &Logger{
		forwardID:  forwardID,
		maxBodyLen: maxBodyLen,
	}

	if logFile == "" {
		// Don't write to stdout - use io.Discard
		// Log entries are delivered via callbacks to the UI
		l.output = io.Discard
	} else {
		// #nosec G304 -- logFile is from config validation, not arbitrary user input
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		l.file = f
		l.output = f
	}

	return l, nil
}

// AddCallback registers a callback to receive log entries
func (l *Logger) AddCallback(cb LogCallback) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callbacks = append(l.callbacks, cb)
}

// ClearCallbacks removes all registered callbacks
func (l *Logger) ClearCallbacks() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callbacks = nil
}

// Log writes a log entry as JSON
func (l *Logger) Log(entry Entry) error {
	entry.ForwardID = l.forwardID
	entry.Timestamp = time.Now()

	// Truncate body if too large
	if len(entry.Body) > l.maxBodyLen {
		entry.Body = entry.Body[:l.maxBodyLen] + "...(truncated)"
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Notify callbacks
	for _, cb := range l.callbacks {
		cb(entry)
	}

	_, err = l.output.Write(append(data, '\n'))
	return err
}

// Close closes the logger
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// GetMaxBodyLen returns the maximum body length for logging
func (l *Logger) GetMaxBodyLen() int {
	return l.maxBodyLen
}
