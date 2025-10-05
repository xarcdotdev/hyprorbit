package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hyprorbit/internal/app/ctl"
)

const (
	ansiPrimary          = "\033[38;5;207m"
	ansiAccent           = "\033[38;5;81m"
	ansiSuccess          = "\033[32m"
	ansiWarning          = "\033[33m"
	ansiPrompt           = "\033[36m"
	ansiReset            = "\033[0m"
	spinnerFrames        = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	waybarConfigFileName = "waybar.yaml"
)

func newInitCommand() *cobra.Command {
	var autoStart bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize hyprorbit configuration and workspace state",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := ctl.FromContext(cmd.Context())
			if err != nil {
				return err
			}

			if err := ensureDaemonRunning(cmd.Context(), client); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if autoStart {
				return resetWorkspaces(cmd.Context(), client, out)
			}

			fmt.Fprintf(out, "\n%s✦ hyprorbit initialization ✦%s\n\n", color(ansiPrimary), color(ansiReset))

			if err := promptConfigGeneration(cmd.Context(), out); err != nil {
				return err
			}

			fmt.Fprintf(out, "%sTip:%s Explore Waybar + keybinding examples at: \n", color(ansiAccent), color(ansiReset))
			fmt.Fprintf(out, "%shttps://github.com/xarcdotdev/hyprorbit/examples%s\n\n", color(ansiAccent), color(ansiReset))

			ok, err := promptYesNo(out, "Reset Hyprland workspaces to match your first orbit/module?", false)
			if err != nil {
				return err
			}
			if ok {
				if err := resetWorkspaces(cmd.Context(), client, out); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(out, "%sSkipping workspace reset – you can run `hyprorbit init` again later.%s\n", color(ansiWarning), color(ansiReset))
			}

			fmt.Fprintf(out, "\n%sAll systems aligned. Welcome to your orbit.%s\n", color(ansiSuccess), color(ansiReset))
			return nil
		},
	}

	cmd.Flags().BoolVar(&autoStart, "autostart", false, "Run workspace reset without prompts or config generation")

	return cmd
}

func promptConfigGeneration(ctx context.Context, out interface{ Write([]byte) (int, error) }) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("init: resolve home directory: %w", err)
	}
	if strings.TrimSpace(homeDir) == "" {
		return fmt.Errorf("init: resolve home directory: empty result")
	}
	configDir := filepath.Join(homeDir, ".config", "hyprorbit")
	configPath := filepath.Join(configDir, "config.yaml")
	if err := ensureDefaultConfigFile(out, configPath); err != nil {
		return err
	}

	waybarPath := filepath.Join(configDir, waybarConfigFileName)
	if err := ensureWaybarConfigFile(out, waybarPath); err != nil {
		return err
	}

	return nil
}

func ensureDefaultConfigFile(out interface{ Write([]byte) (int, error) }, path string) error {
	exists := fileExists(path)
	var question string
	if exists {
		question = fmt.Sprintf("Regenerate default config at %s", path)
	} else {
		question = fmt.Sprintf("Create default config at %s", path)
	}
	create, err := promptYesNo(out, question, !exists)
	if err != nil {
		return err
	}
	if !create {
		fmt.Fprintf(out, "%sKeeping existing configuration.%s\n\n", color(ansiWarning), color(ansiReset))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("init: create config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultConfigYAML), 0o644); err != nil {
		return fmt.Errorf("init: write config: %w", err)
	}
	fmt.Fprintf(out, "%s✓%s Wrote starter config to %s\n\n", color(ansiSuccess), color(ansiReset), path)
	return nil
}

func ensureWaybarConfigFile(out interface{ Write([]byte) (int, error) }, path string) error {
	exists := fileExists(path)
	var question string
	if exists {
		question = fmt.Sprintf("Regenerate Waybar module config at %s", path)
	} else {
		question = fmt.Sprintf("Create Waybar module config at %s", path)
	}
	create, err := promptYesNo(out, question, !exists)
	if err != nil {
		return err
	}
	if !create {
		fmt.Fprintf(out, "%sKeeping existing Waybar configuration.%s\n\n", color(ansiWarning), color(ansiReset))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("init: create Waybar config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultWaybarConfigYAML), 0o644); err != nil {
		return fmt.Errorf("init: write Waybar config: %w", err)
	}
	fmt.Fprintf(out, "%s✓%s Wrote Waybar module config to %s\n\n", color(ansiSuccess), color(ansiReset), path)
	return nil
}

func resetWorkspaces(ctx context.Context, client *ctl.Client, out interface{ Write([]byte) (int, error) }) error {
	spinner := newSpinner(out, "Sweeping workspaces", time.Millisecond*80)
	spinner.Start()
	defer spinner.Stop()

	if err := client.WorkspaceReset(ctx); err != nil {
		return err
	}

	spinner.Update("Aligning orbit")
	if err := client.WorkspaceAlign(ctx); err != nil {
		return err
	}

	spinner.StopWithMessage(fmt.Sprintf("%s✓%s Workspaces reset to orbit baseline", color(ansiSuccess), color(ansiReset)))
	return nil
}

func ensureDaemonRunning(ctx context.Context, client *ctl.Client) error {
	if client == nil {
		return fmt.Errorf("init: daemon client unavailable")
	}
	if err := client.DaemonStatus(ctx); err != nil {
		if isConnectionError(err) {
			return fmt.Errorf("hyprorbit daemon is not running. Start it (e.g. `hyprorbitd`) and rerun this command")
		}
		return err
	}
	return nil
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

func promptYesNo(out interface{ Write([]byte) (int, error) }, question string, defYes bool) (bool, error) {
	var yesOption, noOption string
	if defYes {
		yesOption = "Y"
		noOption = "n"
	} else {
		yesOption = "y"
		noOption = "N"
	}
	prompt := fmt.Sprintf("%s? [%s%s%s/%s%s%s]: ", question, color(ansiPrompt), yesOption, color(ansiReset), color(ansiPrompt), noOption, color(ansiReset))
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return false, err
	}

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, context.Canceled) {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defYes, nil
	}
	return line == "y" || line == "yes", nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type spinnerWriter interface {
	Write([]byte) (int, error)
}

type spinner struct {
	out    spinnerWriter
	msg    string
	delay  time.Duration
	stopCh chan struct{}
	update chan string
}

func newSpinner(out spinnerWriter, msg string, delay time.Duration) *spinner {
	return &spinner{
		out:    out,
		msg:    msg,
		delay:  delay,
		stopCh: make(chan struct{}),
		update: make(chan string, 1),
	}
}

func (s *spinner) Start() {
	go func() {
		frames := []rune(spinnerFrames)
		idx := 0
		for {
			select {
			case <-s.stopCh:
				return
			case msg := <-s.update:
				s.msg = msg
			default:
			}
			frame := string(frames[idx%len(frames)])
			idx++
			fmt.Fprintf(s.out, "%s%s %s%s\r", color(ansiAccent), frame, s.msg, color(ansiReset))
			time.Sleep(s.delay)
		}
	}()
}

func (s *spinner) Update(msg string) {
	select {
	case s.update <- msg:
	default:
	}
}

func (s *spinner) Stop() {
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
	fmt.Fprintf(s.out, "\r%*s\r", 80, "")
}

func (s *spinner) StopWithMessage(message string) {
	s.Stop()
	fmt.Fprintf(s.out, "%s\n", message)
}

const defaultConfigYAML = `# hyprorbit default configuration
orbits:
  - name: "alpha"
    label: "α"
    color: "#BC83F9"
  - name: "beta"
    label: "β"
    color: "#F97583"
  - name: "gamma"
    label: "γ"
    color: "#85E89D"
  - name: "delta"
    label: "δ"
    color: "#f55050ff"
  - name: "epsilon"
    label: "ε"
    color: "#FFAB70"
  - name: "zeta"
    label: "ζ"
    color: "#f5de2aff"
  - name: "theta"
    label: "η"
    color: "#70bcffff"
  - name: "iota"
    label: "ι"
    color: "#e7e7e7ff"

# Module definitions with focus rules
modules:
  code:
    focus:
      match: "class:^code$"
      cmd: ["code"]
    seed:
      - match: "class:^code$"
        cmd: ["code"]
      - match: "class:^(ghostty|Alacritty|kitty)$"
        cmd: ["ghostty"]
  gfx:
    focus:
      match: "class:^(Firefox|zen)$"
      cmd: ["zen-browser"]
    seed:
      - match: "class:^(Firefox|zen)$"
        cmd: ["zen-browser"]
      - match: "class:^Gimp$"
        cmd: ["gimp"]
  comm:
    focus:
      match: "class:^Thunderbird$"
      cmd: ["thunderbird"]
    seed:
      - match: "class:^Thunderbird$"
        cmd: ["thunderbird"]
  media:
    focus:
      match: "class:^(mpv)$"
      cmd: ["mpv"]
  surf:
    focus:
      match: "class:^zen"
      cmd: ["zen-browser"]

defaults:
  float: false
  move: true

orbit:
  switch_preference: last-active-first
`

const defaultWaybarConfigYAML = `module_watch:
  text: ["module", "workspace"]
  tooltip: ["orbit_label", "workspace"]
  alt: ["workspace"]
  class:
    sources: ["module", "orbit"]
`
