package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

//nolint:gochecknoglobals // set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "devback %s\ncommit: %s\nbuilt:  %s\ngo:     %s\nos:     %s/%s\n",
				version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		},
	}
}
