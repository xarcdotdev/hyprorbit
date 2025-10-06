package debug

import (
	"fmt"
	"io"
	"log"
	"os"

	"hyprorbit/internal/config"
)

// ComponentLogger wraps a standard logger with component tagging.
type ComponentLogger struct {
	component string
	logger    *log.Logger
}

// Printf formats and logs a message with the component tag.
func (c *ComponentLogger) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("[%s] %s", c.component, msg)
}

// Print logs a message with the component tag.
func (c *ComponentLogger) Print(msg string) {
	c.logger.Printf("[%s] %s", c.component, msg)
}

// NewLogger creates a debug logger for a specific component based on config.
// Returns nil if debug logging is disabled for the component.
func NewLogger(component string, cfg *config.DebugConfig) (*log.Logger, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	// Check component-specific flag
	switch component {
	case "dispatcher":
		if !cfg.Dispatcher {
			return nil, nil
		}
	case "hyprctl":
		if !cfg.Hyprctl {
			return nil, nil
		}
	default:
		// Unknown component, no logging
		return nil, nil
	}

	logPath := cfg.LogFilePath()
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open debug log file %q: %w", logPath, err)
	}

	// Create logger with no prefix - timestamp will come first
	baseLogger := log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	wrapper := &ComponentLogger{
		component: component,
		logger:    baseLogger,
	}
	wrapper.Printf("Debug logging initialized for %s", component)

	// Return the underlying logger wrapped in our ComponentLogger
	// But we need to return *log.Logger, so we'll use a different approach
	logger := log.New(&componentWriter{component: component, logger: baseLogger}, "", 0)
	logger.Printf("Debug logging initialized for %s", component)
	return logger, nil
}

// componentWriter wraps log output to inject component tag after timestamp.
type componentWriter struct {
	component string
	logger    *log.Logger
}

func (w *componentWriter) Write(p []byte) (n int, err error) {
	// The incoming message already has newline, so we trim it
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	w.logger.Printf("[%s] %s", w.component, msg)
	return len(p), nil
}

// NewMultiLogger creates a logger that writes to multiple outputs.
func NewMultiLogger(component string, writers ...io.Writer) *log.Logger {
	if len(writers) == 0 {
		return nil
	}
	multiWriter := io.MultiWriter(writers...)
	return log.New(multiWriter, fmt.Sprintf("[%s] ", component), log.LstdFlags|log.Lmicroseconds)
}
