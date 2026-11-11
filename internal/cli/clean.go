package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/oplog"
	"github.com/thecloudranger/zap/internal/plan"
	"github.com/thecloudranger/zap/internal/policy"
)

// CleanResult is the `data` payload of `zap clean --json`.
type CleanResult struct {
	DryRun  bool          `json:"dryRun"`
	Plan    plan.Plan     `json:"plan"`
	Results []plan.Result `json:"results"`
}

type cleanFlags struct {
	dryRun, yes, asJSON bool
	target              string
}

const disclaimer = `Before zap deletes anything, please read this once:

  • zap removes git worktrees, and by default their branches.
  • It only acts in repositories that contain a .zap file (see: zap init).
  • It only removes worktrees that pass every safety check: clean, unlocked,
    not protected, not active within min_age, and not where you are standing.
  • Before deleting a branch it saves the tip under refs/zap/backup/, so the
    branch can be restored with: git branch <name> <backup-ref>
  • Files git ignores inside a removed worktree (node_modules, build output,
    .env and other local secrets) are deleted and cannot be restored.

`

func newCleanCmd(d Deps) *cobra.Command {
	var fl cleanFlags
	cmd := &cobra.Command{
		Use:   "clean [PATH]",
		Short: "Remove merged, empty and missing worktrees of an opted-in repository",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fl.dryRun && fl.yes {
				return usageErr(errors.New("--dry-run and --yes cannot be used together"))
			}
			err := runClean(cmd, d, args, fl)
			if err == nil || !fl.asJSON {
				return err
			}
			var ee *ExitError
			if errors.As(err, &ee) && ee.Reported {
				return err
			}
			code := ExitFailure
			if ee != nil {
				code = ee.Code
			}
			body := &ErrorBody{Code: errorCode(err), Message: err.Error()}
			if werr := writeJSON(cmd.OutOrStdout(), Envelope{Command: "clean", Status: "error", Error: body}); werr != nil {
				return errors.Join(err, werr)
			}
			return &ExitError{Code: code, Err: err, Reported: true}
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&fl.dryRun, "dry-run", "n", false, "show what would be removed, change nothing")
	f.BoolVarP(&fl.yes, "yes", "y", false, "do not ask for confirmation (required when not on a terminal)")
	f.BoolVar(&fl.asJSON, "json", false, "output a JSON envelope on stdout")
	f.StringVar(&fl.target, "target", "", "branch that counts as merged-into (overrides .zap)")
	return cmd
}

func runClean(cmd *cobra.Command, d Deps, args []string, fl cleanFlags) error {
	out := cmd.OutOrStdout()
	dir, err := targetDir(d, args)
	if err != nil {
		return err
	}
	cwd, _ := d.Getwd()
	p, err := plan.Build(cmd.Context(), d.Git, dir, plan.Options{Target: fl.target, Now: d.Now(), Cwd: cwd})
	if err != nil {
		return err
	}
	toZap, _ := p.Counts()
	if fl.dryRun || !p.OptedIn || toZap == 0 {
		if fl.asJSON {
			return writeOK(out, "clean", CleanResult{DryRun: true, Plan: p, Results: []plan.Result{}})
		}
		renderPlan(out, p, d.Home)
		if fl.dryRun && toZap > 0 {
			_, _ = fmt.Fprintln(out, "\nDry run: nothing was changed.")
		}
		return nil
	}

	in := bufio.NewReader(d.In)
	if err := ensureDisclaimer(d, in, out, fl); err != nil {
		return err
	}
	if !fl.yes {
		if !d.IsTTY() || fl.asJSON {
			return &ExitError{Code: ExitRefused, Err: errors.New("not on a terminal: pass --yes to confirm, or --dry-run to preview")}
		}
		renderPlan(out, p, d.Home)
		if !ask(in, out, fmt.Sprintf("\nZap %d worktree%s? [y/N] ", toZap, plural(toZap)), "y", "yes") {
			return &ExitError{Code: ExitRefused, Err: errors.New("cancelled; nothing was changed")}
		}
	}

	results, err := plan.Apply(cmd.Context(), d.Git, p, plan.ApplyOptions{Now: d.Now, Cwd: cwd, Log: oplog.Log{Dir: d.StateDir}})
	if err != nil {
		return err
	}
	if fl.asJSON {
		if err := writeOK(out, "clean", CleanResult{Plan: p, Results: results}); err != nil {
			return err
		}
	} else {
		renderResults(out, p, results, d.Home)
	}
	for _, r := range results {
		if r.Outcome == plan.Failed {
			return &ExitError{Code: ExitFailure, Err: errors.New("some worktrees could not be removed"), Reported: fl.asJSON}
		}
	}
	return nil
}

// ensureDisclaimer asks once, on a terminal, and remembers the answer.
func ensureDisclaimer(d Deps, in *bufio.Reader, out io.Writer, fl cleanFlags) error {
	cfg, err := policy.LoadConfig(d.ConfigDir)
	if err != nil {
		return err
	}
	if cfg.Accepted() {
		return nil
	}
	if !d.IsTTY() || fl.asJSON {
		return &ExitError{Code: ExitRefused, Err: errors.New("the one-time disclaimer has not been accepted; run `zap clean` in a terminal once")}
	}
	_, _ = fmt.Fprint(out, disclaimer)
	if !ask(in, out, `Type "zap" to accept: `, "zap") {
		return &ExitError{Code: ExitRefused, Err: errors.New("disclaimer not accepted; nothing was changed")}
	}
	cfg.Disclaimer = policy.Acceptance{Version: policy.DisclaimerVersion, AcceptedAt: d.Now().UTC()}
	if err := policy.SaveConfig(d.ConfigDir, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	return nil
}

func ask(in *bufio.Reader, out io.Writer, prompt string, accept ...string) bool {
	_, _ = fmt.Fprint(out, prompt)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		_, _ = fmt.Fprintln(out)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	for _, a := range accept {
		if answer == a {
			return true
		}
	}
	return false
}

func renderPlan(w io.Writer, p plan.Plan, home string) {
	optIn := "opted in (.zap)"
	if !p.OptedIn {
		optIn = "not opted in: run `zap init` to allow cleaning"
	}
	_, _ = fmt.Fprintf(w, "%s  ·  target %s  ·  %s\n\n", tildify(p.Repo, home), p.Target.Name, optIn)
	if len(p.Items) == 0 {
		_, _ = fmt.Fprintf(w, "Nothing to zap (%d active worktree%s).\n", p.ActiveCount, plural(p.ActiveCount))
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  \tWORKTREE\tBRANCH\tSTATUS\tWHAT")
	for _, it := range p.Items {
		mark, what := "✓", describe(it)
		if it.Action == plan.Keep {
			mark, what = "·", "keep: "+it.Refusal
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", mark, relTo(it.Path, p.Repo, home), dash(it.Branch), it.Status, what)
	}
	_ = tw.Flush()
	toZap, keep := p.Counts()
	_, _ = fmt.Fprintf(w, "\n%d to zap · %d kept · %d active\n", toZap, keep, p.ActiveCount)
}

func describe(it plan.Item) string {
	s := "remove worktree"
	if it.Action == plan.Prune {
		s = "unregister (folder already gone)"
	}
	switch {
	case it.DeleteBranch:
		s += ", delete branch"
	case it.BranchNote != "":
		s += ", branch " + it.BranchNote
	}
	return s
}

func renderResults(w io.Writer, p plan.Plan, results []plan.Result, home string) {
	marks := map[string]string{plan.Zapped: "✓", plan.Skipped: "-", plan.Failed: "✗"}
	zapped := 0
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range results {
		if r.Outcome == plan.Zapped {
			zapped++
		}
		detail := r.Detail
		if r.BranchDeleted {
			detail = strings.TrimSpace("branch deleted, backup " + r.BackupRef + " " + detail)
		}
		_, _ = fmt.Fprintf(tw, "  %s %s\t%s\t%s\n", marks[r.Outcome], r.Outcome, relTo(r.Path, p.Repo, home), detail)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\nZapped %d worktree%s. See `zap history` for the record.\n", zapped, plural(zapped))
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
