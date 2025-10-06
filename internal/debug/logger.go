package debug

import (
	"fmt"
	"io"
	"log"
	"os"

	"hyprorbit/internal/config"
)

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
	default:
		// Unknown component, no logging
		return nil, nil
	}

	logPath := cfg.LogFilePath()
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open debug log file %q: %w", logPath, err)
	}

	logger := log.New(f, fmt.Sprintf("[%s] ", component), log.LstdFlags|log.Lmicroseconds)
	logger.Printf("Debug logging initialized for %s", component)
	return logger, nil
}

// NewMultiLogger creates a logger that writes to multiple outputs.
func NewMultiLogger(component string, writers ...io.Writer) *log.Logger {
	if len(writers) == 0 {
		return nil
	}
	multiWriter := io.MultiWriter(writers...)
	return log.New(multiWriter, fmt.Sprintf("[%s] ", component), log.LstdFlags|log.Lmicroseconds)
}
