package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/leftovers"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/sizer"
)

// CheckResult is the `data` payload of `zap check --json`.
type CheckResult struct {
	Stale      int64    `json:"staleBytes"` // merged or empty worktrees
	StaleCount int      `json:"staleCount"`
	Free       uint64   `json:"freeBytes"`
	Total      uint64   `json:"totalBytes"`
	Leftovers  int64    `json:"leftoverBytes"` // agent temp files and transcripts
	MinStale   int64    `json:"minStaleBytes"`
	MinFree    int64    `json:"minFreeBytes"`
	Attention  bool     `json:"attention"`
	Reasons    []string `json:"reasons,omitempty"`
}

type checkFlags struct {
	minStale, minFree string
	notify, asJSON    bool
}

func newCheckCmd(d Deps) *cobra.Command {
	var fl checkFlags
	cmd := &cobra.Command{
		Use:   "check",
		Short: "One line on merged or empty worktrees and free disk space; exit 3 when either passes a limit",
		Long: "Scan every repository under the scan roots and say how much merged or empty worktrees\n" +
			"take and how much the disk has left. Nothing is deleted.\n\n" +
			"Exits 3 when those worktrees use at least --min-stale or free space is under --min-free,\n" +
			"so it fits cron, a launchd or systemd timer (see `zap schedule`), or a shell prompt.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			minStale, err := sizer.ParseSize(fl.minStale)
			if err != nil {
				return usageErr(fmt.Errorf("--min-stale: %w", err))
			}
			minFree, err := sizer.ParseSize(fl.minFree)
			if err != nil {
				return usageErr(fmt.Errorf("--min-free: %w", err))
			}
			out := cmd.OutOrStdout()
			res, err := check(cmd, d, minStale, minFree, fl.asJSON)
			if fl.asJSON {
				if err != nil {
					return writeErr(out, "check", err)
				}
				if err := writeJSON(out, Envelope{Command: "check", Status: "ok", Data: res}); err != nil {
					return err
				}
			} else {
				if err != nil {
					return err
				}
				renderCheck(out, res)
			}
			if !res.Attention {
				return nil
			}
			if fl.notify && d.Notify != nil {
				d.Notify(cmd.Context(), "zap", checkSummary(res))
			}
			return &ExitError{Code: ExitAttention, Err: errors.New(strings.Join(res.Reasons, "; ")), Reported: true}
		},
	}
	f := cmd.Flags()
	f.StringVar(&fl.minStale, "min-stale", "1GB", "flag merged or empty worktrees once they use this much")
	f.StringVar(&fl.minFree, "min-free", "10GB", "flag the disk once free space drops under this")
	f.BoolVar(&fl.notify, "notify", false, "also show a desktop notification when a limit is passed")
	f.BoolVar(&fl.asJSON, "json", false, "output a JSON envelope on stdout")
	return cmd
}

func check(cmd *cobra.Command, d Deps, minStale, minFree int64, quiet bool) (CheckResult, error) {
	res := CheckResult{MinStale: minStale, MinFree: minFree}
	scanned, err := scanAll(cmd, d, inspect.Options{Sizer: sizer.NewCounter()}, quiet)
	if err != nil {
		return res, err
	}
	for _, rep := range scanned.Repos {
		for _, a := range rep.Worktrees {
			if a.Main || (a.Verdict.Status != model.StatusMerged && a.Verdict.Status != model.StatusEmpty) {
				continue
			}
			res.StaleCount++
			if a.Facts.Size != nil {
				res.Stale += a.Facts.Size.Total
			}
		}
	}
	res.Free, res.Total = scanned.Free, scanned.Total
	if gs, err := leftovers.Find(cmd.Context(), leftovers.Options{Home: d.Home, TmpRoot: d.TmpRoot}); err == nil {
		res.Leftovers = leftovers.Total(gs)
	}
	if res.overStale() {
		res.Reasons = append(res.Reasons, fmt.Sprintf("merged or empty worktrees use %s", sizer.Human(res.Stale)))
	}
	if res.Total > 0 && res.Free < uint64(minFree) {
		res.Reasons = append(res.Reasons, fmt.Sprintf("only %s free", sizer.Human(int64(res.Free))))
	}
	res.Attention = len(res.Reasons) > 0
	return res, nil
}

func (r CheckResult) overStale() bool { return r.StaleCount > 0 && r.Stale >= r.MinStale }

// checkSummary is the one sentence `check` prints and notifies.
func checkSummary(r CheckResult) string {
	var b strings.Builder
	switch r.StaleCount {
	case 0:
		b.WriteString("No merged or empty worktrees.")
	case 1:
		fmt.Fprintf(&b, "1 merged or empty worktree uses %s.", sizer.Human(r.Stale))
	default:
		fmt.Fprintf(&b, "%d merged or empty worktrees use %s.", r.StaleCount, sizer.Human(r.Stale))
	}
	if r.Total > 0 {
		fmt.Fprintf(&b, " The disk has %s free of %s.", sizer.Human(int64(r.Free)), sizer.Human(int64(r.Total)))
	}
	return b.String()
}

func renderCheck(w io.Writer, r CheckResult) {
	_, _ = fmt.Fprintln(w, checkSummary(r))
	if r.Leftovers > 0 {
		_, _ = fmt.Fprintf(w, "Agent temp files and transcripts use %s more; see zap map.\n", sizer.Human(r.Leftovers))
	}
	if r.overStale() {
		_, _ = fmt.Fprintln(w, "Run zap to review them; its safety checks decide what goes.")
	}
}
