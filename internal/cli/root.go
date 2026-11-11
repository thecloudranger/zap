// Package cli wires zap's cobra commands. It is the only package that prints.
package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/version"
)

// Deps are the side-effecting collaborators, injectable for tests.
type Deps struct {
	Git   gitx.Runner
	Out   io.Writer
	Err   io.Writer
	Getwd func() (string, error)
	Home  string // for display only: paths under it are shown as ~/...
}

// DefaultDeps returns the real implementations.
func DefaultDeps() Deps {
	home, _ := os.UserHomeDir()
	return Deps{Git: gitx.Exec{}, Out: os.Stdout, Err: os.Stderr, Getwd: os.Getwd, Home: home}
}

// NewRoot builds the zap command tree.
func NewRoot(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "zap",
		Short:         "Find and zap stale git worktrees left behind by agents and humans",
		Version:       version.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// No subcommand: show help.
			return cmd.Help()
		},
	}
	root.SetOut(d.Out)
	root.SetErr(d.Err)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageErr(err) })
	root.SetVersionTemplate("zap {{.Version}}\n")
	root.AddCommand(newScanCmd(d), newVersionCmd())
	return root
}
