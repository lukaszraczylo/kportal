package logger

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// KlogWriter is an io.Writer that routes klog output through our structured logger.
// It parses klog messages and routes them to appropriate log levels.
// It is thread-safe for concurrent writes.
type KlogWriter struct {
	logger *Logger
	buffer *bytes.Buffer
	mu     sync.Mutex
}

// NewKlogWriter creates a new KlogWriter that routes k8s client-go logs
// through our structured logger.
func NewKlogWriter(logger *Logger) *KlogWriter {
	return &KlogWriter{
		logger: logger,
		buffer: &bytes.Buffer{},
	}
}

// Write implements io.Writer.
// It parses klog output and routes it through our structured logger.
// This method is thread-safe.
func (w *KlogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write to buffer first
	w.buffer.Write(p)

	// Process complete lines
	for {
		line, err := w.buffer.ReadString('\n')
		if err != nil {
			// No complete line yet, write back what we read and wait for more
			if err == io.EOF && line != "" {
				w.buffer.WriteString(line)
			}
			break
		}

		// Process the complete line
		w.processLine(strings.TrimSpace(line))
	}

	return len(p), nil
}

// processLine parses a klog line and routes it to the appropriate log level.
func (w *KlogWriter) processLine(line string) {
	if line == "" {
		return
	}

	// Parse klog format: "I1124 12:34:56.789012   12345 file.go:123] message"
	// First character indicates level: I=Info, W=Warning, E=Error, F=Fatal
	if len(line) < 1 {
		return
	}

	level := line[0]
	message := line

	// Try to extract just the message part after "]"
	if idx := strings.Index(line, "] "); idx != -1 {
		message = line[idx+2:]
	}

	// Determine log level and route accordingly
	switch level {
	case 'I': // Info
		w.logger.Debug(message, map[string]interface{}{
			"source": "k8s-client",
		})
	case 'W': // Warning
		w.logger.Warn(message, map[string]interface{}{
			"source": "k8s-client",
		})
	case 'E', 'F': // Error or Fatal
		w.logger.Error(message, map[string]interface{}{
			"source": "k8s-client",
		})
	default:
		// Unknown format, log as debug
		w.logger.Debug(message, map[string]interface{}{
			"source": "k8s-client",
		})
	}
}
