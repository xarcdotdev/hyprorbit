package main

import (
	"github.com/spf13/cobra"

	"hyprorbit/internal/cli"
	"hyprorbit/internal/cli/presenter"
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
	orbitCmd.AddCommand(newOrbitListCommand())

	return orbitCmd
}

func newOrbitGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the active orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			opts := client.Options()
			record, err := client.OrbitGet(cmd.Context())
			if err != nil {
				return err
			}
			return presenter.PrintOrbit(cmd.OutOrStdout(), opts.PresenterOptions(), record)
		},
	}
}

func newOrbitNextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Switch to the next orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			opts := client.Options()
			record, err := client.OrbitNext(cmd.Context())
			if err != nil {
				return err
			}
			return presenter.PrintOrbit(cmd.OutOrStdout(), opts.PresenterOptions(), record)
		},
	}
}

func newOrbitPrevCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prev",
		Short: "Switch to the previous orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			opts := client.Options()
			record, err := client.OrbitPrev(cmd.Context())
			if err != nil {
				return err
			}
			return presenter.PrintOrbit(cmd.OutOrStdout(), opts.PresenterOptions(), record)
		},
	}
}

func newOrbitSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name>",
		Short: "Activate a specific orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			opts := client.Options()
			record, err := client.OrbitSet(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return presenter.PrintOrbit(cmd.OutOrStdout(), opts.PresenterOptions(), record)
		},
	}
}

func newOrbitListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured orbits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := cli.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			opts := client.Options()
			summaries, err := client.OrbitList(cmd.Context())
			if err != nil {
				return err
			}
			return presenter.PrintOrbitSummaries(cmd.OutOrStdout(), opts.PresenterOptions(), summaries)
		},
	}
}
