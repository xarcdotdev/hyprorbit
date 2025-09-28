package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"hypr-orbits/internal/runtime"
)

func newOrbitCommand() *cobra.Command {
	orbitCmd := &cobra.Command{
		Use:   "orbit",
		Short: "Manage orbit contexts",
	}

	orbitCmd.AddCommand(newOrbitGetCommand())
	orbitCmd.AddCommand(newOrbitNextCommand())
	orbitCmd.AddCommand(newOrbitPrevCommand())
	orbitCmd.AddCommand(newOrbitSetCommand())

	return orbitCmd
}

func newOrbitGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the active orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newOrbitService(cmd.Context())
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			name, err := svc.currentOrbit(ctx)
			if err != nil {
				return runtime.WrapError(err, 1)
			}

			record, err := svc.orbitRecord(ctx, name)
			if err != nil {
				return runtime.WrapError(err, 1)
			}

			if err := printOrbit(cmd, record); err != nil {
				return runtime.WrapError(err, 1)
			}
			return nil
		},
	}
}

func newOrbitNextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Switch to the next orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newOrbitService(cmd.Context())
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			seq, err := svc.sequence(ctx)
			if err != nil {
				return runtime.WrapError(err, 1)
			}
			if len(seq) == 0 {
				return runtime.WrapError(fmt.Errorf("orbit: no orbits configured"), 1)
			}

			current, err := svc.currentOrbit(ctx)
			if err != nil {
				return runtime.WrapError(err, 1)
			}

			idx := indexOf(seq, current)
			if idx == -1 {
				return runtime.WrapError(fmt.Errorf("orbit: current orbit %q not found", current), 1)
			}

			nextName := seq[(idx+1)%len(seq)]
			if err := svc.setOrbit(ctx, nextName); err != nil {
				return runtime.WrapError(err, 1)
			}

			record, err := svc.orbitRecord(ctx, nextName)
			if err != nil {
				return runtime.WrapError(err, 1)
			}

			if err := printOrbit(cmd, record); err != nil {
				return runtime.WrapError(err, 1)
			}
			return nil
		},
	}
}

func newOrbitPrevCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prev",
		Short: "Switch to the previous orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newOrbitService(cmd.Context())
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			seq, err := svc.sequence(ctx)
			if err != nil {
				return runtime.WrapError(err, 1)
			}
			if len(seq) == 0 {
				return runtime.WrapError(fmt.Errorf("orbit: no orbits configured"), 1)
			}

			current, err := svc.currentOrbit(ctx)
			if err != nil {
				return runtime.WrapError(err, 1)
			}

			idx := indexOf(seq, current)
			if idx == -1 {
				return runtime.WrapError(fmt.Errorf("orbit: current orbit %q not found", current), 1)
			}

			prevIdx := idx - 1
			if prevIdx < 0 {
				prevIdx = len(seq) - 1
			}
			prevName := seq[prevIdx]
			if err := svc.setOrbit(ctx, prevName); err != nil {
				return runtime.WrapError(err, 1)
			}

			record, err := svc.orbitRecord(ctx, prevName)
			if err != nil {
				return runtime.WrapError(err, 1)
			}

			if err := printOrbit(cmd, record); err != nil {
				return runtime.WrapError(err, 1)
			}
			return nil
		},
	}
}

func newOrbitSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name>",
		Short: "Activate a specific orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newOrbitService(cmd.Context())
			if err != nil {
				return err
			}

			target := args[0]
			ctx := cmd.Context()

			record, err := svc.orbitRecord(ctx, target)
			if err != nil {
				return runtime.WrapError(err, 2)
			}

			if err := svc.setOrbit(ctx, target); err != nil {
				return runtime.WrapError(err, 1)
			}

			if err := printOrbit(cmd, record); err != nil {
				return runtime.WrapError(err, 1)
			}
			return nil
		},
	}
}

func indexOf(list []string, needle string) int {
	for i, v := range list {
		if v == needle {
			return i
		}
	}
	return -1
}
