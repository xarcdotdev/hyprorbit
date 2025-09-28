package orbit

import (
	"fmt"
	"io"
	"strings"
)

// Print outputs the orbit record in tab-separated format.
func Print(w io.Writer, record *Record) error {
	if record == nil {
		return fmt.Errorf("orbit: nothing to print")
	}
	parts := []string{record.Name}
	if record.Label != "" {
		parts = append(parts, record.Label)
	}
	if record.Color != "" {
		parts = append(parts, record.Color)
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "\t"))
	return err
}
