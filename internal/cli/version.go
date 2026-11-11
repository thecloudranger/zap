package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/version"
)

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Get()
			if asJSON {
				return writeOK(cmd.OutOrStdout(), "version", info)
			}
			s := "zap " + info.Version
			if info.Commit != "" {
				s += " (" + info.Commit
				if info.Date != "" {
					s += ", " + info.Date
				}
				s += ")"
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), s)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}
