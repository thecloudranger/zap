package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/oplog"
	"github.com/thecloudranger/zap/internal/restore"
)

// UndoResult is the `data` payload of `zap undo --json`.
type UndoResult struct {
	Run      string            `json:"run"`
	DryRun   bool              `json:"dryRun"`
	Outcomes []restore.Outcome `json:"outcomes"`
}

func newUndoCmd(d Deps) *cobra.Command {
	var asJSON, dryRun bool
	cmd := &cobra.Command{
		Use:   "undo [RUN]",
		Short: "Bring back the branches and worktrees removed by a clean run (default: the latest)",
		Long: "Bring back the branches and worktrees removed by a clean run (default: the latest).\n\n" +
			"Branches are recreated from their backup refs and worktrees are re-added at their old\n" +
			"paths. Files git ignores (node_modules, build output, .env) cannot be restored.",
		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			run := ""
			if len(args) == 1 {
				run = args[0]
			}
			run, outs, err := restore.Undo(cmd.Context(), d.Git, oplog.Log{Dir: d.StateDir},
				restore.Options{Run: run, DryRun: dryRun, Now: d.Now})
			if errors.Is(err, restore.ErrNothingToUndo) {
				if asJSON {
					return writeOK(out, "undo", UndoResult{Run: run, DryRun: dryRun, Outcomes: []restore.Outcome{}})
				}
				_, _ = fmt.Fprintln(out, "Nothing to undo.")
				return nil
			}
			if err != nil {
				if asJSON {
					return writeErr(out, "undo", err)
				}
				return err
			}
			if outs == nil {
				outs = []restore.Outcome{}
			}
			if asJSON {
				if err := writeOK(out, "undo", UndoResult{Run: run, DryRun: dryRun, Outcomes: outs}); err != nil {
					return err
				}
			} else {
				renderUndo(cmd, run, outs, dryRun, d.Home)
			}
			for _, o := range outs {
				if o.Result == oplog.RestoreFailed {
					return &ExitError{Code: ExitFailure, Err: errors.New("some worktrees could not be restored"), Reported: asJSON}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be restored, change nothing")
	return cmd
}

func renderUndo(cmd *cobra.Command, run string, outs []restore.Outcome, dryRun bool, home string) {
	w := cmd.OutOrStdout()
	verb := "Restoring"
	if dryRun {
		verb = "Would restore"
	}
	_, _ = fmt.Fprintf(w, "%s run %s\n\n", verb, run)
	for _, o := range outs {
		mark := "↺"
		if o.Result == oplog.RestoreFailed {
			mark = "✗"
		}
		_, _ = fmt.Fprintf(w, "  %s %s\n", mark, tildify(o.Path, home))
		for _, s := range o.Steps {
			_, _ = fmt.Fprintf(w, "      %s\n", s)
		}
		if o.Detail != "" {
			_, _ = fmt.Fprintf(w, "      error: %s\n", strings.TrimSpace(o.Detail))
		}
	}
	if dryRun {
		_, _ = fmt.Fprintln(w, "\nDry run: nothing was changed.")
		return
	}
	_, _ = fmt.Fprintln(w, "\nFiles git ignores (node_modules, build output, .env) were not restored.")
}
