package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"hyprorbits/internal/app/ctl"
	"hyprorbits/internal/module"
	"hyprorbits/internal/runtime"
)

func newModuleCommand() *cobra.Command {
	moduleCmd := &cobra.Command{
		Use:   "module",
		Short: "Interact with module workspaces",
	}

	moduleCmd.AddCommand(newModuleGetCommand())
	moduleCmd.AddCommand(newModuleJumpCommand())
	moduleCmd.AddCommand(newModuleFocusCommand())
	moduleCmd.AddCommand(newModuleSeedCommand())
	moduleCmd.AddCommand(newModuleListCommand())

	return moduleCmd
}

func newModuleGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print details about the current module workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			status, err := client.ModuleGet(cmd.Context())
			if err != nil {
				return err
			}
			return ctl.PrintModuleStatus(cmd.OutOrStdout(), client.Options(), status)
		},
	}
}

func newModuleJumpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "jump <module>",
		Short: "Jump to a module workspace in the active orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			res, err := client.ModuleJump(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return ctl.PrintModule(cmd.OutOrStdout(), client.Options(), res)
		},
	}
}

func newModuleFocusCommand() *cobra.Command {
	var (
		matchExpr string
		spawnCmd  []string
		floatWin  bool
		noMove    bool
	)

	cmd := &cobra.Command{
		Use:   "focus <module>",
		Short: "Focus or launch a module workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if matchExpr != "" {
				if _, err := module.ParseMatcher(matchExpr); err != nil {
					return runtime.WrapError(err, 2)
				}
			}

			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			res, err := client.ModuleFocus(cmd.Context(), args[0], ctl.ModuleFocusOptions{
				Matcher:    matchExpr,
				Command:    spawnCmd,
				ForceFloat: floatWin,
				NoMove:     noMove,
			})
			if err != nil {
				return err
			}
			return ctl.PrintModule(cmd.OutOrStdout(), client.Options(), res)
		},
	}

	cmd.Flags().StringVar(&matchExpr, "match", "", "Override matcher in field=regex form")
	cmd.Flags().StringSliceVar(&spawnCmd, "cmd", nil, "Command to spawn when no client matches")
	cmd.Flags().BoolVar(&floatWin, "float", false, "Force spawned window to float")
	cmd.Flags().BoolVar(&noMove, "no-move", false, "Prevent moving clients between workspaces")

	return cmd
}

func newModuleSeedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seed <module>",
		Short: "Populate a module workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}
			results, err := client.ModuleSeed(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return ctl.PrintModuleList(cmd.OutOrStdout(), client.Options(), results)
		},
	}
}

func newModuleListCommand() *cobra.Command {
	var (
		flagActive   bool
		flagInactive bool
		flagAll      bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List module workspaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := "all"
			selected := 0
			if flagActive {
				filter = "active"
				selected++
			}
			if flagInactive {
				filter = "inactive"
				selected++
			}
			if flagAll {
				filter = "all"
				selected++
			}
			if selected > 1 {
				return fmt.Errorf("specify at most one of --active, --inactive, or --all")
			}

			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			summaries, err := client.ModuleList(cmd.Context(), filter)
			if err != nil {
				return err
			}
			return ctl.PrintWorkspaceSummaries(cmd.OutOrStdout(), client.Options(), summaries)
		},
	}

	cmd.Flags().BoolVar(&flagActive, "active", false, "Show configured workspaces that currently exist")
	cmd.Flags().BoolVar(&flagInactive, "inactive", false, "Show configured workspaces that do not exist")
	cmd.Flags().BoolVar(&flagAll, "all", false, "Show all workspaces (default)")

	return cmd
}
