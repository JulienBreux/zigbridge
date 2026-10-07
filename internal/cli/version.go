package cli

import (
	"github.com/julienbreux/zigbridge/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version and build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printF(cmd.OutOrStdout(), "zigbridge version %s (commit: %s, built: %s)\n",
				version.Version, version.Commit, version.BuildDate)
			return nil
		},
	}
}
