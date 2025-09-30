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
	ansiPrimary   = "\033[38;5;207m"
	ansiAccent    = "\033[38;5;81m"
	ansiSuccess   = "\033[32m"
	ansiWarning   = "\033[33m"
	ansiPrompt    = "\033[36m"
	ansiReset     = "\033[0m"
	spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
)

func newInitCommand() *cobra.Command {
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
	defaultPath := filepath.Join(homeDir, ".config", "hyprorbit", "config.yaml")
	exists := fileExists(defaultPath)
	var question string
	if exists {
		question = fmt.Sprintf("Regenerate default config at %s", defaultPath)
	} else {
		question = fmt.Sprintf("Create default config at %s", defaultPath)
	}
	create, err := promptYesNo(out, question, !exists)
	if err != nil {
		return err
	}
	if !create {
		fmt.Fprintf(out, "%sKeeping existing configuration.%s\n\n", color(ansiWarning), color(ansiReset))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		return fmt.Errorf("init: create config directory: %w", err)
	}
	if err := os.WriteFile(defaultPath, []byte(defaultConfigYAML), 0o644); err != nil {
		return fmt.Errorf("init: write config: %w", err)
	}
	fmt.Fprintf(out, "%s✓%s Wrote starter config to %s\n\n", color(ansiSuccess), color(ansiReset), defaultPath)
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

const defaultConfigYAML = `orbits:
  - name: "alpha"
    label: "α"
  - name: "beta"
    label: "β"

modules:
  code:
    focus:
      match: "class=.*Code"
      cmd: ["kitty", "-T", "Code"]
  comm:
    focus:
      match: "title=.*Slack"
      cmd: ["flatpak", "run", "com.slack.Slack"]

defaults:
  float: false
  move: true

waybar:
  module_watch:
    text: ["module", "workspace"]
    tooltip: ["orbit_label", "workspace"]
    alt: ["workspace"]
    class:
      sources: ["module", "orbit"]
`
