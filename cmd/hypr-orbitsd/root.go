package main

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/cobra"
)

func execute() int {
	root := newRootCommand()
	root.SetContext(context.Background())
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	var (
		cfgPath      string
		socketPath   string
		logLevel     string
		logFormat    string
		cacheTTL     time.Duration
		disableCache bool
	)

	cmd := &cobra.Command{
		Use:           "hypr-orbitsd",
		Short:         "Stateful orbit/module daemon for Hyprland",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("hypr-orbitsd daemon not yet implemented")
		},
	}

	cmd.PersistentFlags().StringVar(&cfgPath, "config", "", "Path to config file")
	cmd.PersistentFlags().StringVar(&socketPath, "socket", "", "Override IPC socket path")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Set log level")
	cmd.PersistentFlags().StringVar(&logFormat, "log-format", "auto", "Set log format (auto,json,text)")
	cmd.PersistentFlags().DurationVar(&cacheTTL, "cache-ttl", 150*time.Millisecond, "Hyprctl client cache TTL")
	cmd.PersistentFlags().BoolVar(&disableCache, "no-cache", false, "Disable hyprctl client caching")

	cmd.MarkFlagsMutuallyExclusive("cache-ttl", "no-cache")

	return cmd
}
