package main

import (
	"bufio"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"hyprorbit/internal/app/ctl"
	"hyprorbit/internal/app/service"
	"hyprorbit/internal/module"
	"hyprorbit/internal/runtime"
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
		Use:   "jump <module|next|prev>",
		Short: "Jump to a module workspace in the active orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			arg := args[0]
			var res *module.Result
			switch arg {
			case "next":
				res, err = client.ModuleJumpNext(cmd.Context())
			case "prev":
				res, err = client.ModuleJumpPrev(cmd.Context())
			default:
				res, err = client.ModuleJump(cmd.Context(), arg)
			}
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
	var (
		flagWaybar       bool
		flagWaybarConfig string
	)

	cmd := &cobra.Command{
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

			var formatter *moduleWatchFormatter
			if !opts.JSON && !opts.Quiet {
				formatter, err = newModuleWatchFormatter(cmd.Context(), moduleWatchFormatterOptions{
					Waybar:           flagWaybar,
					ConfigPath:       opts.ConfigPath,
					WaybarConfigPath: flagWaybarConfig,
				})
				if err != nil {
					return err
				}
			}

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

				if formatter == nil {
					continue
				}

				payload, err := formatter.Format(snapshot)
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

	cmd.Flags().BoolVar(&flagWaybar, "waybar", false, "Emit Waybar-compatible JSON envelope")
	cmd.Flags().StringVar(&flagWaybarConfig, "waybar-config", "", "Override Waybar config file path")

	return cmd
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
