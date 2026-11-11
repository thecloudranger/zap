package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/oplog"
)

// HistoryResult is the `data` payload of `zap history --json`.
type HistoryResult struct {
	Entries []oplog.Entry `json:"entries"`
}

func newHistoryCmd(d Deps) *cobra.Command {
	var (
		asJSON bool
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show what zap removed, newest first",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be >= 0"))
			}
			entries, skipped, err := oplog.Log{Dir: d.StateDir}.Read()
			if err != nil {
				if asJSON {
					return writeErr(out, "history", err)
				}
				return err
			}
			if limit > 0 && len(entries) > limit {
				entries = entries[:limit]
			}
			if entries == nil {
				entries = []oplog.Entry{}
			}
			if asJSON {
				env := Envelope{Command: "history", Status: "ok", Data: HistoryResult{Entries: entries}}
				if skipped > 0 {
					env.Warnings = []string{fmt.Sprintf("%d unreadable log line(s) skipped", skipped)}
				}
				return writeJSON(out, env)
			}
			if len(entries) == 0 {
				_, _ = fmt.Fprintln(out, "zap has not removed anything yet.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "TIME\tRUN\tRESULT\tWORKTREE\tBRANCH\tBACKUP")
			for _, e := range entries {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Time.Local().Format("2006-01-02 15:04"), e.Run, e.Result,
					tildify(e.Path, d.Home), dash(e.Branch), dash(e.BackupRef))
			}
			_ = tw.Flush()
			if skipped > 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d unreadable log line(s) skipped\n", skipped)
			}
			_, _ = fmt.Fprintln(out, "\nUndo the latest run with `zap undo`, or a specific one with `zap undo <RUN>`.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	cmd.Flags().IntVar(&limit, "limit", 20, "show at most N entries (0 = all)")
	return cmd
}
