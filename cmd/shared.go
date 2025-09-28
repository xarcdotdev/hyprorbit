package cmd

import (
	"github.com/spf13/cobra"

	"hypr-orbits/internal/runtime"
)

func requireRuntime(cmd *cobra.Command) (*runtime.Runtime, error) {
	rt, err := runtime.FromContext(cmd.Context())
	if err != nil {
		return nil, runtime.WrapError(err, 1)
	}
	return rt, nil
}
