package main

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"hyprorbits/internal/app/ctl"
	"hyprorbits/internal/runtime"
)

var colorEnabled = true

func execute() int {
	root := newRootCommand()
	root.SetContext(context.Background())
	if err := root.Execute(); err != nil {
		return runtime.ExitCodeFromError(err)
	}
	return 0
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "hyprorbits",
		Short:         "Orbit-focused workspace orchestration for Hyprland",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if _, err := ctl.FromContext(ctx); err == nil {
				return nil
			}

			socket, _ := cmd.Flags().GetString("socket")
			jsonOut, _ := cmd.Flags().GetBool("json")
			quiet, _ := cmd.Flags().GetBool("quiet")
			flagNoColor, _ := cmd.Flags().GetBool("no-color")
			envNoColor := strings.TrimSpace(os.Getenv("NO_COLOR")) != ""
			noColor := flagNoColor || envNoColor
			colorEnabled = !noColor

			client := ctl.NewClient(ctl.Options{
				SocketPath: socket,
				JSON:       jsonOut,
				Quiet:      quiet,
				NoColor:    noColor,
			})

			cmd.SetContext(ctl.WithClient(ctx, client))
			return nil
		},
	}

	root.PersistentFlags().String("socket", "", "Override IPC socket path")
	root.PersistentFlags().Bool("json", false, "Emit JSON responses")
	root.PersistentFlags().Bool("quiet", false, "Suppress output on success")
	root.PersistentFlags().Bool("no-color", false, "Disable ANSI colors in output")

	root.AddCommand(newOrbitCommand())
	root.AddCommand(newModuleCommand())
	root.AddCommand(newDaemonCommand())
	root.AddCommand(newInitCommand())

	return root
}

func color(code string) string {
	if !colorEnabled {
		return ""
	}
	return code
}
