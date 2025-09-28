package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"hyprorbits/internal/app/service"
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
		Use:           "hyprorbitsd",
		Short:         "Stateful orbit/module daemon for Hyprland",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd.Context(), collectOptions(cmd.Flags(), cfgPath, socketPath, logLevel, logFormat, cacheTTL, disableCache))
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

func collectOptions(flags *pflag.FlagSet, cfgPath, socketPath, logLevel, logFormat string, cacheTTL time.Duration, disableCache bool) service.Options {
	if flags != nil {
		cfgPath, _ = flags.GetString("config")
		socketPath, _ = flags.GetString("socket")
		logLevel, _ = flags.GetString("log-level")
		logFormat, _ = flags.GetString("log-format")
		cacheTTL, _ = flags.GetDuration("cache-ttl")
		disableCache, _ = flags.GetBool("no-cache")
	}

	return service.Options{
		ConfigPath:   cfgPath,
		SocketPath:   socketPath,
		LogLevel:     logLevel,
		LogFormat:    logFormat,
		CacheTTL:     cacheTTL,
		DisableCache: disableCache,
	}
}

func runServer(ctx context.Context, opts service.Options) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv, err := service.NewServer(ctx, opts)
	if err != nil {
		return err
	}
	return srv.Serve(ctx)
}
