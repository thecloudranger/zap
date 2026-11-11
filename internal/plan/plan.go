// Package plan decides what `zap clean` will do (Build) and does it (Apply).
// The same Plan drives --dry-run, --json and the real run, so a preview cannot
// drift from what actually happens.
package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/policy"
	"github.com/thecloudranger/zap/internal/procs"
	"github.com/thecloudranger/zap/internal/safety"
	"github.com/thecloudranger/zap/internal/sizer"
	"github.com/thecloudranger/zap/internal/verdict"
)

// Action is what happens to one candidate.
type Action string

// Actions.
const (
	Remove Action = "remove" // git worktree remove
	Prune  Action = "prune"  // folder already gone: unregister only
	Keep   Action = "keep"   // refused by a gate or not opted in
)

// Item is one candidate worktree: its status is listed in the repo's rules.
type Item struct {
	Path         string              `json:"path"`
	Branch       string              `json:"branch,omitempty"`
	Head         string              `json:"head"`
	Status       model.Status        `json:"status"`
	Reason       string              `json:"reason"`
	Action       Action              `json:"action"`
	DeleteBranch bool                `json:"deleteBranch"`
	BranchNote   string              `json:"branchNote,omitempty"`
	Refusal      string              `json:"refusal,omitempty"` // why it is kept
	Eligible     bool                `json:"eligible"`          // passed every safety gate
	Gates        []model.CheckResult `json:"gates"`
	Size         *model.Usage        `json:"size,omitempty"`
}

// Plan is everything clean would do in one repository.
type Plan struct {
	Repo        string       `json:"repo"`
	CommonDir   string       `json:"-"`
	Target      model.Target `json:"target"`
	OptedIn     bool         `json:"optedIn"`
	Policy      policy.Repo  `json:"policy"`
	Items       []Item       `json:"items"`
	ActiveCount int          `json:"activeCount"` // worktrees that are not candidates at all
	Warnings    []string     `json:"warnings,omitempty"`
}

// Options control Build.
type Options struct {
	Target string // overrides .zap target
	Fetch  bool   // fetch remotes before deciding
	Sizer  *sizer.Counter
	Now    time.Time
	Cwd    string
	Gates  []safety.Gate // nil means safety.Default()
	// Procs lists running processes; nil means procs.List.
	Procs func(context.Context) ([]procs.Proc, error)
}

// Build inspects the repository containing dir and plans its cleanup.
func Build(ctx context.Context, git gitx.Runner, dir string, opt Options) (Plan, error) {
	repo, err := gitx.ListRepo(ctx, git, dir)
	if err != nil {
		return Plan{}, err
	}
	pol, found, err := policy.LoadRepo(repo.Root)
	if err != nil {
		return Plan{}, err
	}
	if !found {
		pol = policy.Defaults()
	}
	target := opt.Target
	if target == "" {
		target = pol.Target
	}
	running := snapshot(ctx, opt.Procs) // before zap starts git processes of its own
	rep, err := inspect.Repo(ctx, git, repo.Root, inspect.Options{Target: target, Fetch: opt.Fetch, Sizer: opt.Sizer})
	if err != nil {
		return Plan{}, err
	}
	gates := opt.Gates
	if gates == nil {
		gates = safety.Default()
	}
	p := Plan{Repo: rep.Root, CommonDir: rep.CommonDir, Target: rep.Target, OptedIn: found, Policy: pol, Warnings: rep.Warnings}
	for _, a := range rep.Worktrees {
		if a.Main {
			continue
		}
		if !pol.Allows(a.Verdict.Status) {
			p.ActiveCount++
			continue
		}
		p.Items = append(p.Items, planItem(ctx, git, rep, a, pol, gates, found, opt, running))
	}
	return p, nil
}

// snapshot lists running processes once for a whole plan.
func snapshot(ctx context.Context, list func(context.Context) ([]procs.Proc, error)) *safety.Procs {
	if list == nil {
		list = procs.List
	}
	ps, err := list(ctx)
	return &safety.Procs{List: ps, Err: err}
}

func planItem(ctx context.Context, git gitx.Runner, rep model.RepoReport, a model.Assessment, pol policy.Repo, gates []safety.Gate, optedIn bool, opt Options, running *safety.Procs) Item {
	it := Item{Path: a.Path, Branch: a.Branch, Head: a.Head, Status: a.Verdict.Status, Reason: a.Verdict.Reason, Size: a.Facts.Size}
	results, ok := safety.Evaluate(&safety.Subject{A: a, Target: rep.Target, Policy: pol, Now: opt.Now, Cwd: opt.Cwd, Procs: running}, gates)
	it.Gates, it.Eligible = results, ok
	switch {
	case !ok:
		it.Action, it.Refusal = Keep, firstFailure(results)
	case !optedIn:
		it.Action, it.Refusal = Keep, "no "+policy.FileName+" in this repo (run `zap init` to opt in)"
	case a.Prunable:
		it.Action = Prune
	default:
		it.Action = Remove
	}
	it.DeleteBranch, it.BranchNote = branchDecision(ctx, git, rep, a, pol)
	return it
}

func firstFailure(rs []model.CheckResult) string {
	for _, r := range rs {
		if r.Outcome != model.Pass && r.Outcome != model.Skipped {
			return r.Detail
		}
	}
	return "refused"
}

// branchDecision deletes a branch only when its work is provably safe: the status
// already says so (merged, empty), or, for a missing worktree, a merge check matches.
func branchDecision(ctx context.Context, git gitx.Runner, rep model.RepoReport, a model.Assessment, pol policy.Repo) (bool, string) {
	switch {
	case a.Branch == "":
		return false, ""
	case pol.DeleteBranch == nil || !*pol.DeleteBranch:
		return false, "kept: delete_branch = false"
	case a.Verdict.Status == model.StatusMerged || a.Verdict.Status == model.StatusEmpty:
		return true, ""
	}
	s := &verdict.Subject{Git: git, Repo: model.Repo{Root: rep.Root}, WT: a.Worktree, Target: rep.Target}
	s.WT.Prunable = false // judge the branch, not the folder
	v := verdict.Evaluate(ctx, s, verdict.MergeRules(), false)
	if v.Status == model.StatusMerged || v.Status == model.StatusEmpty {
		return true, ""
	}
	return false, fmt.Sprintf("kept: branch has commits not in %s", rep.Target.Name)
}

// Reclaimable is the measured disk use of the worktrees the plan would remove.
func (p Plan) Reclaimable() int64 {
	var n int64
	for _, it := range p.Items {
		if it.Action != Keep && it.Size != nil {
			n += it.Size.Total
		}
	}
	return n
}

// Counts summarizes a plan.
func (p Plan) Counts() (zap, keep int) {
	for _, it := range p.Items {
		if it.Action == Keep {
			keep++
		} else {
			zap++
		}
	}
	return zap, keep
}

// Describe says what removing the item does, e.g. "remove worktree, delete branch".
func (it Item) Describe() string {
	s := "remove worktree"
	if it.Action == Prune {
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
