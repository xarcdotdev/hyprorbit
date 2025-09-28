package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"hypr-orbits/internal/runtime"
)

// Execute runs the CLI. It is invoked by main.
func Execute() int {
	root := NewRootCommand()
	root.SetContext(context.Background())
	if err := root.Execute(); err != nil {
		// Cobra already prints the error message when SilenceErrors is false.
		return runtime.ExitCodeFromError(err)
	}
	return 0
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "hypr-orbits",
		Short:         "Orbit-focused workspace orchestration for Hyprland",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if runtime.HasRuntime(ctx) {
				return nil
			}

			cfgPath, _ := cmd.Flags().GetString("config")
			verbose, _ := cmd.Flags().GetBool("verbose")

			opts := runtime.Options{
				ConfigPath: cfgPath,
				Verbose:    verbose,
			}

			rt, err := runtime.Bootstrap(ctx, opts)
			if err != nil {
				return err
			}

			cmd.SetContext(runtime.WithRuntime(ctx, rt))
			return nil
		},
	}

	root.PersistentFlags().StringP("config", "c", "", "Path to config file")
	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")

	root.AddCommand(newOrbitCommand())
	root.AddCommand(newModuleCommand())

	return root
}

// helper for main to exit with proper status code
func ExecuteOrExit() {
	if code := Execute(); code != 0 {
		os.Exit(code)
	}
}
