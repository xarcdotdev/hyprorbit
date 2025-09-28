package cmd

import (
	"github.com/spf13/cobra"

	"hypr-orbits/internal/runtime"
)

func newModuleCommand() *cobra.Command {
	moduleCmd := &cobra.Command{
		Use:   "module",
		Short: "Interact with module workspaces",
	}

	moduleCmd.AddCommand(newModuleJumpCommand())
	moduleCmd.AddCommand(newModuleFocusCommand())
	moduleCmd.AddCommand(newModuleSeedCommand())

	return moduleCmd
}

func newModuleJumpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "jump <module>",
		Short: "Jump to a module workspace in the active orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := requireRuntime(cmd); err != nil {
				return err
			}
			return runtime.ErrNotImplemented
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
			if _, err := requireRuntime(cmd); err != nil {
				return err
			}
			return runtime.ErrNotImplemented
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
			if _, err := requireRuntime(cmd); err != nil {
				return err
			}
			return runtime.ErrNotImplemented
		},
	}
}
