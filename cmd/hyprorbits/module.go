package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"hyprorbits/internal/app/ctl"
	"hyprorbits/internal/app/service"
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
	moduleCmd.AddCommand(newModuleWatchCommand())

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

func newModuleWatchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Stream module status updates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			stream, err := client.ModuleWatch(cmd.Context())
			if err != nil {
				return err
			}
			defer stream.Close()

			opts := client.Options()
			scanner := bufio.NewScanner(stream)
			scanner.Buffer(make([]byte, 0, 4096), 256*1024)
			writer := cmd.OutOrStdout()

			for scanner.Scan() {
				if opts.Quiet {
					continue
				}
				line := scanner.Bytes()

				if opts.JSON {
					if _, err := fmt.Fprintln(writer, string(line)); err != nil {
						return err
					}
					continue
				}

				var snapshot service.StatusSnapshot
				if err := json.Unmarshal(line, &snapshot); err != nil {
					return fmt.Errorf("module watch: decode snapshot: %w", err)
				}

				payload, err := marshalWaybarSnapshot(snapshot)
				if err != nil {
					return err
				}

				if _, err := fmt.Fprintln(writer, string(payload)); err != nil {
					return err
				}
			}

			if err := scanner.Err(); err != nil && cmd.Context().Err() == nil {
				return fmt.Errorf("module watch: stream: %w", err)
			}
			return nil
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

func marshalWaybarSnapshot(snapshot service.StatusSnapshot) ([]byte, error) {
	text := snapshot.Module
	if text == "" {
		text = snapshot.Workspace
	}

	payload := map[string]any{
		"text":      text,
		"workspace": snapshot.Workspace,
		"module":    snapshot.Module,
	}

	tooltip := snapshot.Workspace
	if tooltip == "" && snapshot.Orbit != nil && snapshot.Orbit.Name != "" {
		tooltip = snapshot.Orbit.Name
	}
	if snapshot.Orbit != nil && snapshot.Orbit.Label != "" {
		tooltip = snapshot.Orbit.Label
	}
	if tooltip != "" {
		payload["tooltip"] = tooltip
	}

	if snapshot.Workspace != "" {
		payload["alt"] = snapshot.Workspace
	}

	classes := make([]string, 0, 3)
	if snapshot.Module != "" {
		classes = append(classes, snapshot.Module)
	}
	if snapshot.Orbit != nil && snapshot.Orbit.Name != "" {
		classes = append(classes, snapshot.Orbit.Name)
		payload["orbit"] = snapshot.Orbit.Name
	}
	if len(classes) > 0 {
		payload["class"] = strings.Join(classes, " ")
	}

	if snapshot.Orbit != nil {
		payload["orbit_record"] = snapshot.Orbit
		if snapshot.Orbit.Label != "" {
			payload["orbit_label"] = snapshot.Orbit.Label
		}
		if snapshot.Orbit.Color != "" {
			payload["color"] = snapshot.Orbit.Color
		}
	}

	return json.Marshal(payload)
}
