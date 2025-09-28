package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// printSequence outputs a space-separated list of names.
func printSequence(cmd *cobra.Command, names []string) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.Join(names, " "))
	return err
}
