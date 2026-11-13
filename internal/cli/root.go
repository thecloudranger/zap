// Package cli wires zap's cobra commands. It is the only package that prints.
package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/notify"
	"github.com/thecloudranger/zap/internal/version"
)

// Deps are the side-effecting collaborators, injectable for tests.
type Deps struct {
	Git       gitx.Runner
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
	IsTTY     func() bool // true when zap may ask the user questions
	Getwd     func() (string, error)
	Now       func() time.Time
	Home      string // for display only: paths under it are shown as ~/...
	ConfigDir string // holds config.toml
	StateDir  string // holds the operations log
	TmpRoot   string // where agents keep temp sessions; empty means /tmp
	// Notify shows a desktop notification and reports whether one was shown; nil shows none.
	Notify func(ctx context.Context, title, body string) bool
	// Exec runs a system command (launchctl, systemctl) for `zap schedule`; nil runs the real one.
	Exec func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DefaultDeps returns the real implementations.
func DefaultDeps() Deps {
	home, _ := os.UserHomeDir()
	return Deps{
		Git:   gitx.Exec{},
		In:    os.Stdin,
		Out:   os.Stdout,
		Err:   os.Stderr,
		IsTTY: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) },
		Getwd: os.Getwd,
		Now:   time.Now,
		Home:  home, ConfigDir: configDir(home), StateDir: stateDir(home),
		Notify: notify.New().Send,
		// ZAP_AGENT_TMP points zap at another temp root, for demos and tests that must not
		// show the real machine's agent sessions.
		TmpRoot: os.Getenv("ZAP_AGENT_TMP"),
	}
}

func configDir(home string) string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "zap")
	}
	return filepath.Join(home, ".config", "zap")
}

func stateDir(home string) string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "zap")
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "zap")
	}
	return filepath.Join(home, ".local", "state", "zap")
}

// NewRoot builds the zap command tree.
func NewRoot(d Deps) *cobra.Command {
	var all, gh bool
	root := &cobra.Command{
		Use:           "zap",
		Short:         "Find and zap stale git worktrees left behind by agents and humans",
		Version:       version.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          usageArgs(cobra.NoArgs),
		Long: "Find and zap stale git worktrees left behind by agents and humans.\n\n" +
			"On a terminal, zap with no command opens an interactive picker.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !d.IsTTY() {
				return cmd.Help()
			}
			return runPicker(cmd, d, all, gh)
		},
	}
	root.Flags().BoolVarP(&all, "all", "a", false, "pick from every repository under the scan roots")
	root.Flags().BoolVar(&gh, "gh", false, "also ask GitHub (gh CLI) for merged pull requests")
	root.SetIn(d.In)
	root.SetOut(d.Out)
	root.SetErr(d.Err)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageErr(err) })
	root.SetVersionTemplate("zap {{.Version}}\n")
	root.AddCommand(newScanCmd(d), newCheckCmd(d), newScheduleCmd(d), newMapCmd(d), newExplainCmd(d), newInitCmd(d), newCleanCmd(d), newHistoryCmd(d), newUndoCmd(d), newRootsCmd(d), newVersionCmd())
	return root
}
