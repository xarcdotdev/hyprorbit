package cmd

import (
	"fmt"
	"strings"

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
			ctx := cmd.Context()
			svc, err := newModuleService(ctx)
			if err != nil {
				return err
			}

			moduleName := args[0]
			if _, ok := svc.moduleRecord(moduleName); !ok {
				return runtime.WrapError(fmt.Errorf("module %q not configured (available: %s)", moduleName, strings.Join(svc.moduleNames(), ", ")), 2)
			}

			res, err := svc.jumpModule(ctx, moduleName)
			if err != nil {
				return runtime.WrapError(err, 1)
			}
			if err := printModuleResult(cmd, res); err != nil {
				return runtime.WrapError(err, 1)
			}
			return nil
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
			ctx := cmd.Context()
			svc, err := newModuleService(ctx)
			if err != nil {
				return err
			}

			moduleName := args[0]
			if _, ok := svc.moduleRecord(moduleName); !ok {
				return runtime.WrapError(fmt.Errorf("module %q not configured (available: %s)", moduleName, strings.Join(svc.moduleNames(), ", ")), 2)
			}

			if matchExpr != "" {
				if _, err := parseMatcherString(matchExpr); err != nil {
					return runtime.WrapError(err, 2)
				}
			}

			opts := focusOptions{
				MatcherOverride: matchExpr,
				CmdOverride:     spawnCmd,
				ForceFloat:      floatWin,
				NoMove:          noMove,
			}

			res, err := svc.focusModule(ctx, moduleName, opts)
			if err != nil {
				return runtime.WrapError(err, 1)
			}
			if err := printModuleResult(cmd, res); err != nil {
				return runtime.WrapError(err, 1)
			}
			return nil
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
			ctx := cmd.Context()
			svc, err := newModuleService(ctx)
			if err != nil {
				return err
			}

			moduleName := args[0]
			if _, ok := svc.moduleRecord(moduleName); !ok {
				return runtime.WrapError(fmt.Errorf("module %q not configured (available: %s)", moduleName, strings.Join(svc.moduleNames(), ", ")), 2)
			}

			results, err := svc.seedModule(ctx, moduleName)
			if err != nil {
				return runtime.WrapError(err, 1)
			}
			for _, res := range results {
				if err := printModuleResult(cmd, res); err != nil {
					return runtime.WrapError(err, 1)
				}
			}
			return nil
		},
	}
}
