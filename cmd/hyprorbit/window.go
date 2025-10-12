package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"hyprorbit/internal/app/ctl"
)

func newWindowCommand() *cobra.Command {
	windowCmd := &cobra.Command{
		Use:   "window",
		Short: "Manipulate windows",
	}

	windowCmd.AddCommand(newWindowMoveCommand())
	windowCmd.AddCommand(newWindowListCommand())

	return windowCmd
}

func newWindowMoveCommand() *cobra.Command {
	var silent bool
	var global bool

	cmd := &cobra.Command{
		Use:   "move <window> <target>",
		Short: "Move a window to a module workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			orbitTarget, moduleTarget, err := parseWindowMoveTarget(args[1])
			if err != nil {
				return err
			}

			results, err := client.WindowMove(cmd.Context(), args[0], orbitTarget, moduleTarget, silent, global)
			if err != nil {
				return err
			}
			return ctl.PrintWindowMoves(cmd.OutOrStdout(), client.Options(), results)
		},
	}

	cmd.Flags().BoolVar(&silent, "silent", false, "Do not focus the target workspace after moving the window")
	cmd.Flags().BoolVar(&global, "global", false, "Select windows from all orbits instead of current orbit only")

	return cmd
}

func newWindowListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List windows with their module and orbit assignments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			windows, err := client.WindowMoveList(cmd.Context())
			if err != nil {
				return err
			}
			return ctl.PrintWindowList(cmd.OutOrStdout(), client.Options(), windows)
		},
	}
}

func parseWindowMoveTarget(raw string) (orbitTarget, moduleTarget string, err error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", fmt.Errorf("window move: target cannot be empty")
	}

	orbitSpec := ""
	moduleSpec := value
	if parts := strings.SplitN(value, "/", 2); len(parts) == 2 {
		orbitSpec = strings.TrimSpace(parts[0])
		moduleSpec = strings.TrimSpace(parts[1])
		if moduleSpec == "" {
			return "", "", fmt.Errorf("window move: module selector missing")
		}
	}

	moduleTarget = ensureModuleTarget(moduleSpec)
	if moduleTarget == "" {
		return "", "", fmt.Errorf("window move: module selector missing")
	}

	if orbitSpec != "" {
		orbitTarget = ensureOrbitTarget(orbitSpec)
		if orbitTarget == "" {
			return "", "", fmt.Errorf("window move: orbit selector missing")
		}
	}

	return orbitTarget, moduleTarget, nil
}

func ensureModuleTarget(spec string) string {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "module:") {
		return trimmed
	}
	return "module:" + trimmed
}

func ensureOrbitTarget(spec string) string {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "orbit:") {
		return trimmed
	}
	return "orbit:" + trimmed
}
