package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/oplog"
	"github.com/thecloudranger/zap/internal/procs"
	"github.com/thecloudranger/zap/internal/safety"
)

// Result outcomes, shared with the operations log.
const (
	Zapped  = oplog.Zapped
	Skipped = oplog.Skipped
	Failed  = oplog.Failed
)

// Result is what happened to one item.
type Result struct {
	Repo          string `json:"repo"`
	Path          string `json:"path"`
	Branch        string `json:"branch,omitempty"`
	Outcome       string `json:"outcome"`
	Detail        string `json:"detail,omitempty"`
	BackupRef     string `json:"backupRef,omitempty"`
	BranchDeleted bool   `json:"branchDeleted"`
	Freed         int64  `json:"freed"` // measured size of the removed worktree, 0 if unknown
}

// ApplyOptions control Apply.
type ApplyOptions struct {
	Run   string // groups the log entries of one clean; empty generates one from Now
	Now   func() time.Time
	Cwd   string
	Log   oplog.Log
	Gates []safety.Gate                               // nil means safety.Default()
	Procs func(context.Context) ([]procs.Proc, error) // nil means procs.List
}

// RunID names a clean run; it is also a path component of the backup refs.
func RunID(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// BackupPrefix is where branch tips are saved before deletion.
const BackupPrefix = "refs/zap/backup/"

// Apply executes the Remove and Prune items of p. Before touching each worktree it
// re-inspects the repository and skips anything that changed since the plan was built.
func Apply(ctx context.Context, git gitx.Runner, p Plan, opt ApplyOptions) ([]Result, error) {
	if !p.OptedIn {
		return nil, fmt.Errorf("plan: %s has not opted in", p.Repo)
	}
	run := opt.Run
	if run == "" {
		run = RunID(opt.Now())
	}
	running := snapshot(ctx, opt.Procs)
	fresh, err := inspect.Repo(ctx, git, p.Repo, inspect.Options{Target: p.Target.Name})
	if err != nil {
		return nil, fmt.Errorf("plan: recheck: %w", err)
	}
	current := map[string]model.Assessment{}
	for _, a := range fresh.Worktrees {
		current[a.Path] = a
	}
	gates := opt.Gates
	if gates == nil {
		gates = safety.Default()
	}

	var results []Result
	for _, it := range p.Items {
		if it.Action == Keep {
			continue
		}
		if err := ctx.Err(); err != nil {
			return results, err
		}
		r := applyItem(ctx, git, p, it, current, gates, run, opt, running)
		results = append(results, r)
		entry := oplog.Entry{
			Run: run, Time: opt.Now(), Repo: p.Repo, Path: it.Path, Branch: it.Branch, Head: it.Head,
			Status: string(it.Status), BackupRef: r.BackupRef, BranchDeleted: r.BranchDeleted,
			Result: r.Outcome, Detail: r.Detail,
		}
		if err := opt.Log.Append(entry); err != nil {
			return results, err // never keep deleting without a record
		}
	}
	return results, nil
}

func applyItem(ctx context.Context, git gitx.Runner, p Plan, it Item, current map[string]model.Assessment, gates []safety.Gate, run string, opt ApplyOptions, running *safety.Procs) Result {
	r := Result{Repo: p.Repo, Path: it.Path, Branch: it.Branch}
	if why := changed(it, current); why != "" {
		r.Outcome, r.Detail = Skipped, "changed since the plan: "+why
		return r
	}
	a := current[it.Path]
	if _, ok := safety.Evaluate(&safety.Subject{A: a, Target: p.Target, Policy: p.Policy, Now: opt.Now(), Cwd: opt.Cwd,
		Procs: running, UnlockStale: p.UnlockStale}, gates); !ok {
		r.Outcome, r.Detail = Skipped, "a safety gate no longer passes"
		return r
	}

	name := it.Branch
	if name == "" {
		name = "detached-" + shortSHA(it.Head)
	}
	r.BackupRef = BackupPrefix + run + "/" + name
	if err := gitx.UpdateRef(ctx, git, p.Repo, r.BackupRef, it.Head); err != nil {
		r.BackupRef = ""
		r.Outcome, r.Detail = Failed, "backup ref: "+err.Error()
		return r
	}

	if it.Unlock && a.Locked { // a stale lock the user asked to release
		if err := gitx.Unlock(ctx, git, p.Repo, it.Path); err != nil {
			r.Outcome, r.Detail = Failed, "unlock: "+err.Error()
			return r
		}
	}
	var err error
	if it.Action == Prune {
		err = gitx.RemoveAdmin(p.CommonDir, it.Path)
	} else {
		err = gitx.RemoveWorktree(ctx, git, p.Repo, it.Path)
	}
	if err != nil {
		r.Outcome, r.Detail = Failed, err.Error()
		return r
	}
	r.Outcome = Zapped
	if it.Size != nil && it.Action == Remove {
		r.Freed = it.Size.Total
	}

	if it.DeleteBranch && it.Branch != "" {
		if err := gitx.DeleteBranch(ctx, git, p.Repo, it.Branch); err != nil {
			r.Detail = "branch kept: " + err.Error()
		} else {
			r.BranchDeleted = true
		}
	} else if it.BranchNote != "" {
		r.Detail = "branch " + it.BranchNote
	}
	return r
}

// changed compares the fresh assessment with the planned one.
func changed(it Item, current map[string]model.Assessment) string {
	a, ok := current[it.Path]
	switch {
	case !ok:
		return "no longer a worktree"
	case a.Head != it.Head:
		return fmt.Sprintf("HEAD moved %s → %s", shortSHA(it.Head), shortSHA(a.Head))
	case a.Branch != it.Branch:
		return fmt.Sprintf("branch changed %q → %q", it.Branch, a.Branch)
	case a.Verdict.Status != it.Status:
		return fmt.Sprintf("status changed %s → %s", it.Status, a.Verdict.Status)
	case a.Prunable != (it.Action == Prune):
		return "folder appeared or disappeared"
	}
	return ""
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
