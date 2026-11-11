package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/thecloudranger/zap/internal/oplog"
	"github.com/thecloudranger/zap/internal/plan"
	"github.com/thecloudranger/zap/internal/policy"
	"github.com/thecloudranger/zap/internal/sizer"
)

// CleanResult is the `data` payload of `zap clean --json`.
type CleanResult struct {
	DryRun  bool          `json:"dryRun"`
	Plans   []plan.Plan   `json:"plans"`
	Results []plan.Result `json:"results"`
}

type cleanFlags struct {
	dryRun, yes, asJSON, fetch, all, unlockStale, gh bool
	target                                           string
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
		Short: "Remove merged, empty and missing worktrees of opted-in repositories",
		Long: "Remove merged, empty and missing worktrees of an opted-in repository (the current one,\n" +
			"or PATH), or of every opted-in repository under the scan roots with --all.",
		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fl.dryRun && fl.yes {
				return usageErr(errors.New("--dry-run and --yes cannot be used together"))
			}
			if fl.all && len(args) > 0 {
				return usageErr(errors.New("--all and PATH cannot be used together"))
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
	f.BoolVar(&fl.fetch, "fetch", false, "fetch all remotes first so merged pull requests are seen")
	f.BoolVarP(&fl.all, "all", "a", false, "every opted-in repository under the scan roots")
	f.BoolVar(&fl.unlockStale, "unlock-stale", false, "release locks whose recorded pid is no longer running")
	f.BoolVar(&fl.gh, "gh", false, "also ask GitHub (gh CLI) for merged pull requests")
	return cmd
}

func runClean(cmd *cobra.Command, d Deps, args []string, fl cleanFlags) error {
	out := cmd.OutOrStdout()
	cwd, _ := d.Getwd()
	plans, warnings, err := buildPlans(cmd, d, args, fl, cwd)
	if err != nil {
		return err
	}
	toZap, repos := zapCounts(plans)
	if fl.dryRun || toZap == 0 {
		if fl.asJSON {
			return writeJSON(out, Envelope{Command: "clean", Status: "ok", Warnings: warnings,
				Data: CleanResult{DryRun: true, Plans: plans, Results: []plan.Result{}}})
		}
		renderPlans(out, plans, warnings, fl.all, d.Home)
		if fl.dryRun && toZap > 0 {
			_, _ = fmt.Fprintf(out, "\nDry run: nothing was changed. Zapping would free about %s.\n", sizer.Human(reclaimable(plans)))
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
		renderPlans(out, plans, warnings, fl.all, d.Home)
		where := ""
		if repos > 1 {
			where = fmt.Sprintf(" in %d repositories", repos)
		}
		if !ask(in, out, fmt.Sprintf("\nZap %d worktree%s%s? [y/N] ", toZap, plural(toZap), where), "y", "yes") {
			return &ExitError{Code: ExitRefused, Err: errors.New("cancelled; nothing was changed")}
		}
	}

	return applyAndReport(cmd, d, plans, warnings, fl, cwd)
}

// applyAndReport applies every opted-in plan under one run id and reports the results.
// clean and the interactive picker both end here.
func applyAndReport(cmd *cobra.Command, d Deps, plans []plan.Plan, warnings []string, fl cleanFlags, cwd string) error {
	out := cmd.OutOrStdout()
	results := []plan.Result{}
	freeBefore, freeErr := sizer.Free(plans[0].Repo)
	run := plan.RunID(d.Now()) // one run across all repositories, so one undo restores it
	for _, p := range plans {
		if n, _ := p.Counts(); n == 0 || !p.OptedIn {
			continue
		}
		rs, err := plan.Apply(cmd.Context(), d.Git, p, plan.ApplyOptions{Run: run, Now: d.Now, Cwd: cwd, Log: oplog.Log{Dir: d.StateDir}})
		results = append(results, rs...)
		if err != nil {
			return err
		}
	}
	if fl.asJSON {
		if err := writeJSON(out, Envelope{Command: "clean", Status: "ok", Warnings: warnings,
			Data: CleanResult{Plans: plans, Results: results}}); err != nil {
			return err
		}
	} else {
		renderResults(out, results, d.Home)
		freeAfter, err := sizer.Free(plans[0].Repo)
		renderFreed(out, results, freeBefore, freeAfter, freeErr == nil && err == nil && !fl.all)
	}
	for _, r := range results {
		if r.Outcome == plan.Failed {
			return &ExitError{Code: ExitFailure, Err: errors.New("some worktrees could not be removed"), Reported: fl.asJSON}
		}
	}
	return nil
}

// buildPlans plans the current repository, PATH, or (with --all) every repository
// under the scan roots that has worktrees. A repository that fails to plan becomes a
// warning in --all mode.
func buildPlans(cmd *cobra.Command, d Deps, args []string, fl cleanFlags, cwd string) ([]plan.Plan, []string, error) {
	opt := plan.Options{Target: fl.target, Fetch: fl.fetch, Now: d.Now(), Cwd: cwd, Sizer: sizer.NewCounter(),
		UnlockStale: fl.unlockStale, GH: ghRunner(fl.gh)}
	if !fl.all {
		dir, err := targetDir(d, args)
		if err != nil {
			return nil, nil, err
		}
		p, err := plan.Build(cmd.Context(), d.Git, dir, opt)
		if err != nil {
			return nil, nil, err
		}
		return []plan.Plan{p}, p.Warnings, nil
	}

	prog := newProgress(cmd, d, fl.asJSON)
	_, found, err := discoverRepos(cmd, d, prog)
	if err != nil {
		prog.done()
		return nil, nil, err
	}
	warnings := found.Warnings
	built := make([]*plan.Plan, len(found.Repos))
	errs := make([]error, len(found.Repos))
	g, ctx := errgroup.WithContext(cmd.Context())
	g.SetLimit(4)
	for i, dir := range found.Repos {
		g.Go(func() error {
			p, err := plan.Build(ctx, d.Git, dir, opt)
			if err != nil && ctx.Err() != nil {
				return ctx.Err()
			}
			built[i], errs[i] = &p, err
			prog.repo(len(found.Repos))
			return nil
		})
	}
	err = g.Wait()
	prog.done()
	if err != nil {
		return nil, nil, err
	}
	var plans []plan.Plan
	for i, p := range built {
		switch {
		case errs[i] != nil:
			warnings = append(warnings, fmt.Sprintf("%s: skipped: %v", tildify(found.Repos[i], d.Home), errs[i]))
		case len(p.Items) > 0:
			plans = append(plans, *p)
			warnings = append(warnings, prefixed(tildify(p.Repo, d.Home), p.Warnings)...)
		}
	}
	return plans, warnings, nil
}

func reclaimable(plans []plan.Plan) int64 {
	var n int64
	for _, p := range plans {
		if p.OptedIn {
			n += p.Reclaimable()
		}
	}
	return n
}

// renderFreed reports space freed: the measured size of what was removed and, when all
// of it was on one disk, the change in that disk's free space.
func renderFreed(w io.Writer, results []plan.Result, before, after uint64, oneDisk bool) {
	var freed int64
	for _, r := range results {
		freed += r.Freed
	}
	if freed == 0 {
		return
	}
	line := fmt.Sprintf("Freed about %s", sizer.Human(freed))
	if oneDisk {
		line += "  ·  free space " + sizer.Human(int64(after))
		if after > before { // other processes write too, so the change can be noise
			line += " (+" + sizer.Human(int64(after-before)) + ")"
		}
	}
	_, _ = fmt.Fprintln(w, line+".")
}

func prefixed(prefix string, ws []string) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = prefix + ": " + w
	}
	return out
}

// zapCounts returns how many worktrees would be removed and in how many repositories.
func zapCounts(plans []plan.Plan) (worktrees, repos int) {
	for _, p := range plans {
		if n, _ := p.Counts(); n > 0 && p.OptedIn {
			worktrees += n
			repos++
		}
	}
	return worktrees, repos
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

func renderPlans(w io.Writer, plans []plan.Plan, warnings []string, all bool, home string) {
	for _, warn := range warnings {
		_, _ = fmt.Fprintf(w, "warning: %s\n", warn)
	}
	if len(warnings) > 0 {
		_, _ = fmt.Fprintln(w)
	}
	if !all {
		for _, p := range plans {
			renderPlan(w, p, home)
		}
		return
	}
	var waiting []plan.Plan
	for _, p := range plans {
		if p.OptedIn {
			renderPlan(w, p, home)
			_, _ = fmt.Fprintln(w)
		} else if len(p.Items) > 0 {
			waiting = append(waiting, p)
		}
	}
	if len(waiting) > 0 {
		_, _ = fmt.Fprintln(w, "Not opted in (run `zap init <repo>` to allow cleaning):")
		for _, p := range waiting {
			n := len(p.Items)
			_, _ = fmt.Fprintf(w, "  %s  ·  %d candidate%s, %d passing the safety checks now\n",
				tildify(p.Repo, home), n, plural(n), eligible(p))
		}
		_, _ = fmt.Fprintln(w)
	}
	toZap, repos := zapCounts(plans)
	if toZap == 0 && len(waiting) == 0 {
		_, _ = fmt.Fprintln(w, "Nothing to zap.")
		return
	}
	_, _ = fmt.Fprintf(w, "%d to zap in %d opted-in repositor%s.\n", toZap, repos, pluralY(repos))
}

func eligible(p plan.Plan) int {
	n := 0
	for _, it := range p.Items {
		if it.Eligible {
			n++
		}
	}
	return n
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
	_, _ = fmt.Fprintln(tw, "  \tWORKTREE\tBRANCH\tSTATUS\tSIZE\tWHAT")
	for _, it := range p.Items {
		mark, what := "✓", it.Describe()
		size := "-"
		if it.Size != nil {
			size = sizer.Human(it.Size.Total)
		}
		if it.Action == plan.Keep {
			mark, what = "·", "keep: "+it.Refusal
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", mark, relTo(it.Path, p.Repo, home), dash(it.Branch), it.Status, size, what)
	}
	_ = tw.Flush()
	toZap, keep := p.Counts()
	line := fmt.Sprintf("\n%d to zap · %d kept · %d active", toZap, keep, p.ActiveCount)
	if n := p.Reclaimable(); n > 0 {
		line += fmt.Sprintf("  ·  frees about %s", sizer.Human(n))
	}
	_, _ = fmt.Fprintln(w, line)
}

func renderResults(w io.Writer, results []plan.Result, home string) {
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
		_, _ = fmt.Fprintf(tw, "  %s %s\t%s\t%s\n", marks[r.Outcome], r.Outcome, relTo(r.Path, r.Repo, home), detail)
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
