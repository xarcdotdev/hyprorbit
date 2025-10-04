package main

import (
	"github.com/spf13/cobra"

	"hyprorbit/internal/app/ctl"
)

func newWindowCommand() *cobra.Command {
	windowCmd := &cobra.Command{
		Use:   "window",
		Short: "Manipulate windows",
	}

	windowCmd.AddCommand(newWindowMoveCommand())

	return windowCmd
}

func newWindowMoveCommand() *cobra.Command {
	var silent bool

	cmd := &cobra.Command{
		Use:   "move <window> <target>",
		Short: "Move a window to a module workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			res, err := client.WindowMove(cmd.Context(), args[0], args[1], silent)
			if err != nil {
				return err
			}
			return ctl.PrintWindowMove(cmd.OutOrStdout(), client.Options(), res)
		},
	}

	cmd.Flags().BoolVar(&silent, "silent", false, "Do not focus the target workspace after moving the window")

	return cmd
}
