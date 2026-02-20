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
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// logBufferPool is used to reuse byte buffers for JSON encoding.
// This reduces allocations when serializing log entries.
var logBufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

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

// stringBuilderPool provides reusable string builders for body truncation.
// This reduces allocations when building truncated body strings.
var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// truncateBody truncates a body string to maxLen, adding a suffix if truncated.
// Uses a pooled buffer to avoid allocations during truncation.
func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}

	// Use pooled buffer for truncation
	buf := stringBuilderPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer stringBuilderPool.Put(buf)

	// Write truncated content
	buf.WriteString(body[:maxLen])
	buf.WriteString("...(truncated)")
	return buf.String()
}

// Log writes a log entry as JSON using a pooled buffer to reduce allocations.
func (l *Logger) Log(entry Entry) error {
	entry.ForwardID = l.forwardID
	entry.Timestamp = time.Now()

	// Truncate body if too large using pooled buffer
	if len(entry.Body) > l.maxBodyLen {
		entry.Body = truncateBody(entry.Body, l.maxBodyLen)
	}

	// Get a buffer from the pool
	buf := logBufferPool.Get().(*bytes.Buffer)
	buf.Reset() // Clear any previous content
	defer logBufferPool.Put(buf)

	// Encode JSON directly into the pooled buffer
	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(entry); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Notify callbacks
	for _, cb := range l.callbacks {
		cb(entry)
	}

	_, err := l.output.Write(buf.Bytes())
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
