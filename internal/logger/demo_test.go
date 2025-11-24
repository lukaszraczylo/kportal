package logger_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/nvm/kportal/internal/logger"
)

// This test demonstrates the logger output formats
func TestLoggerDemo(t *testing.T) {
	t.Skip("Demo only - run manually with: go test -v -run TestLoggerDemo")

	fmt.Println("\n=== TEXT FORMAT (DEFAULT) ===")
	textBuf := &bytes.Buffer{}
	textLogger := logger.New(logger.LevelInfo, logger.FormatText, textBuf)

	textLogger.Info("Port forward started", map[string]interface{}{
		"forward_id": "prod/default/pod/app:8080",
		"local_port": 8080,
		"pod":        "app-xyz123",
	})

	textLogger.Warn("Connection failed, retrying", map[string]interface{}{
		"forward_id": "prod/default/pod/app:8080",
		"error":      "connection refused",
		"retry":      3,
	})

	textLogger.Error("Failed to resolve resource", map[string]interface{}{
		"forward_id": "prod/default/pod/app:8080",
		"error":      "pod not found",
	})

	fmt.Print(textBuf.String())

	fmt.Println("\n=== JSON FORMAT ===")
	jsonBuf := &bytes.Buffer{}
	jsonLogger := logger.New(logger.LevelInfo, logger.FormatJSON, jsonBuf)

	jsonLogger.Info("Port forward started", map[string]interface{}{
		"forward_id": "prod/default/pod/app:8080",
		"local_port": 8080,
		"pod":        "app-xyz123",
	})

	jsonLogger.Warn("Connection failed, retrying", map[string]interface{}{
		"forward_id": "prod/default/pod/app:8080",
		"error":      "connection refused",
		"retry":      3,
	})

	jsonLogger.Error("Failed to resolve resource", map[string]interface{}{
		"forward_id": "prod/default/pod/app:8080",
		"error":      "pod not found",
	})

	fmt.Print(jsonBuf.String())

	fmt.Println("\n=== LOG LEVEL FILTERING (Debug level disabled) ===")
	filteredBuf := &bytes.Buffer{}
	filteredLogger := logger.New(logger.LevelInfo, logger.FormatText, filteredBuf)

	filteredLogger.Debug("This will not appear", nil)
	filteredLogger.Info("This will appear", nil)
	filteredLogger.Warn("This will also appear", nil)

	fmt.Print(filteredBuf.String())
}
