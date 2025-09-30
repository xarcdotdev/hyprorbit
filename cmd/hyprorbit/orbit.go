package main

import (
	"github.com/spf13/cobra"

	"hyprorbit/internal/app/ctl"
)

func newOrbitCommand() *cobra.Command {
	orbitCmd := &cobra.Command{
		Use:   "orbit",
		Short: "Manage orbit contexts",
	}

	orbitCmd.AddCommand(newOrbitGetCommand())
	orbitCmd.AddCommand(newOrbitNextCommand())
	orbitCmd.AddCommand(newOrbitPrevCommand())
	orbitCmd.AddCommand(newOrbitSetCommand())

	return orbitCmd
}

func newOrbitGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the active orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			record, err := client.OrbitGet(cmd.Context())
			if err != nil {
				return err
			}
			return ctl.PrintOrbit(cmd.OutOrStdout(), client.Options(), record)
		},
	}
}

func newOrbitNextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Switch to the next orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			record, err := client.OrbitNext(cmd.Context())
			if err != nil {
				return err
			}
			return ctl.PrintOrbit(cmd.OutOrStdout(), client.Options(), record)
		},
	}
}

func newOrbitPrevCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prev",
		Short: "Switch to the previous orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			record, err := client.OrbitPrev(cmd.Context())
			if err != nil {
				return err
			}
			return ctl.PrintOrbit(cmd.OutOrStdout(), client.Options(), record)
		},
	}
}

func newOrbitSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name>",
		Short: "Activate a specific orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			record, err := client.OrbitSet(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return ctl.PrintOrbit(cmd.OutOrStdout(), client.Options(), record)
		},
	}
}
