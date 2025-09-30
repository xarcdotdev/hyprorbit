package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"hyprorbit/internal/app/ctl"
)

func newDaemonCommand() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Control the hyprorbit daemon",
	}

	daemonCmd.AddCommand(newDaemonReloadCommand())
	return daemonCmd
}

func newDaemonReloadCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload daemon configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			if err := client.DaemonReload(cmd.Context()); err != nil {
				return err
			}

			opts := client.Options()
			if opts.Quiet {
				return nil
			}
			if opts.JSON {
				payload := map[string]string{"status": "reloaded"}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				return enc.Encode(payload)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "daemon reloaded")
			return err
		},
	}
}
