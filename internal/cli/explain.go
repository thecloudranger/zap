package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/sizer"
)

// ErrNotWorktree is returned when explain's PATH is not a worktree of the repository.
var ErrNotWorktree = errors.New("not a worktree of this repository")

// ExplainResult is the `data` payload of `zap explain --json`.
type ExplainResult struct {
	Root     string           `json:"root"`
	Target   model.Target     `json:"target"`
	Worktree model.Assessment `json:"worktree"`
}

func newExplainCmd(d Deps) *cobra.Command {
	var (
		asJSON, fetch, gh bool
		target            string
	)
	cmd := &cobra.Command{
		Use:   "explain PATH",
		Short: "Show every check behind a worktree's status",
		Long: "Show every check behind a worktree's status.\n\n" +
			"For a worktree whose folder is gone, run this from inside the repository.",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			res, err := explain(cmd, d, args[0], target, fetch, gh)
			if asJSON {
				if err != nil {
					return writeErr(out, "explain", err)
				}
				return writeOK(out, "explain", res)
			}
			if err != nil {
				return err
			}
			return renderExplain(out, res, d.Home, d.Now())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	cmd.Flags().StringVar(&target, "target", "", "branch that counts as merged-into (default: origin/HEAD, main, master)")
	cmd.Flags().BoolVar(&fetch, "fetch", false, "fetch all remotes first so merged pull requests are seen")
	cmd.Flags().BoolVar(&gh, "gh", false, "also ask GitHub (gh CLI) for merged pull requests")
	return cmd
}

func explain(cmd *cobra.Command, d Deps, arg, target string, fetch, gh bool) (ExplainResult, error) {
	path, err := filepath.Abs(arg)
	if err != nil {
		return ExplainResult{}, err
	}
	dir := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path, dir = real, real
		if fi, err := os.Stat(real); err == nil && !fi.IsDir() {
			dir = filepath.Dir(real) // git needs a directory to run in
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// A missing worktree can only be found through the repository we are in.
		if dir, err = d.Getwd(); err != nil {
			return ExplainResult{}, fmt.Errorf("cli: working directory: %w", err)
		}
	}
	rep, err := inspect.Repo(cmd.Context(), d.Git, dir, inspect.Options{Target: target, AllRules: true, Fetch: fetch, Sizer: sizer.NewCounter(), GH: ghRunner(gh)})
	if err != nil {
		return ExplainResult{}, err
	}
	for _, a := range rep.Worktrees {
		if a.Path == path {
			return ExplainResult{Root: rep.Root, Target: rep.Target, Worktree: a}, nil
		}
	}
	return ExplainResult{}, fmt.Errorf("cli: %s: %w %s", path, ErrNotWorktree, rep.Root)
}

var outcomeMark = map[model.Outcome]string{
	model.Match:   "✓",
	model.NoMatch: "✗",
	model.Skipped: "-",
	model.Errored: "!",
}

func renderExplain(w io.Writer, res ExplainResult, home string, now time.Time) error {
	a := res.Worktree
	_, _ = fmt.Fprintf(w, "%s\n", tildify(a.Path, home))
	_, _ = fmt.Fprintf(w, "  repo     %s\n", tildify(res.Root, home))
	_, _ = fmt.Fprintf(w, "  branch   %s @ %s\n", branchLabel(a.Worktree), short(a.Head))
	_, _ = fmt.Fprintf(w, "  target   %s (%s)\n", res.Target.Name, strings.Join(res.Target.Refs, ", "))
	_, _ = fmt.Fprintf(w, "\n  status   %s — %s\n", strings.ToUpper(string(a.Verdict.Status)), a.Verdict.Reason)
	if len(a.Verdict.Checks) > 0 {
		_, _ = fmt.Fprintln(w, "\n  checks (first ✓ decides)")
		for _, c := range a.Verdict.Checks {
			_, _ = fmt.Fprintf(w, "    %s %-12s %s\n", outcomeMark[c.Outcome], c.Check, c.Detail)
		}
	}
	f := a.Facts
	_, _ = fmt.Fprintln(w, "\n  facts")
	_, _ = fmt.Fprintf(w, "    dirty          %s\n", yesno(f.Dirty, fmt.Sprintf("yes, %d changed path%s", f.Changes, plural(f.Changes))))
	_, _ = fmt.Fprintf(w, "    locked         %s\n", yesno(a.Locked, "yes "+a.LockReason))
	if u := f.Size; u != nil {
		_, _ = fmt.Fprintf(w, "    size           %s freed if removed (%s of it dependencies and build output)\n", sizer.Human(u.Total), sizer.Human(u.Deps))
		if u.Shared > 0 {
			_, _ = fmt.Fprintf(w, "    shared         %s hard-linked with other folders, not freed\n", sizer.Human(u.Shared))
		}
	}
	_, _ = fmt.Fprintf(w, "    created        %s\n", stamp(now, f.Created))
	_, _ = fmt.Fprintf(w, "    last commit    %s\n", stamp(now, f.LastCommit))
	_, _ = fmt.Fprintf(w, "    last activity  %s\n", stamp(now, f.LastActivity))
	for _, e := range a.Errors {
		_, _ = fmt.Fprintf(w, "    ! %s\n", e)
	}
	return nil
}

func yesno(b bool, yes string) string {
	if b {
		return strings.TrimSpace(yes)
	}
	return "no"
}

func stamp(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	rel := ago(now, t) + " ago"
	if rel == "now ago" {
		rel = "just now"
	}
	return fmt.Sprintf("%s (%s)", rel, t.Local().Format("2006-01-02 15:04"))
}
