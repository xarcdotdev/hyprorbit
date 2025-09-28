package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func printOrbit(cmd *cobra.Command, record *orbitRecord) error {
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
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.Join(parts, "\t"))
	return err
}

func printSequence(cmd *cobra.Command, names []string) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.Join(names, " "))
	return err
}
