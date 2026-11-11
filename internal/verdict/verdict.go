// Package verdict decides a worktree's status with an ordered chain of checks.
// The first check that matches decides; a check that errors never matches.
package verdict

import (
	"context"
	"fmt"
	"strings"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
)

// Subject is what a check looks at.
type Subject struct {
	Git    gitx.Runner
	Repo   model.Repo
	WT     model.Worktree
	Target model.Target
}

// head is the commit to judge: the branch when there is one, else the detached HEAD.
func (s *Subject) head() string {
	if s.WT.Branch != "" {
		return "refs/heads/" + s.WT.Branch
	}
	return s.WT.Head
}

// Check is one rule in the chain. Adding a heuristic means adding a type and a Rule.
type Check interface {
	Name() string
	Eval(ctx context.Context, s *Subject) model.CheckResult
}

// Rule pairs a check with the status it decides when it matches.
type Rule struct {
	Check  Check
	Status model.Status
}

// Default is the chain used by scan and explain, cheapest first.
func Default() []Rule {
	return []Rule{
		{Missing{}, model.StatusMissing},
		{Empty{}, model.StatusEmpty},
		{Ancestor{}, model.StatusMerged},
		{EmptyDiff{}, model.StatusMerged},
	}
}

// Evaluate runs the chain. With all=false it stops at the first match (scan);
// with all=true every check runs so explain can show the full picture.
func Evaluate(ctx context.Context, s *Subject, rules []Rule, all bool) model.Verdict {
	if s.WT.Main {
		return model.Verdict{Status: model.StatusMain, Reason: "main worktree of the repository"}
	}
	if s.WT.Branch != "" && s.WT.Branch == s.Target.Name {
		return model.Verdict{Status: model.StatusActive, Reason: fmt.Sprintf("checked out on the target branch %q", s.Target.Name)}
	}
	v := model.Verdict{Status: model.StatusActive}
	for _, r := range rules {
		if v.Reason != "" && !all {
			break
		}
		res := r.Check.Eval(ctx, s)
		res.Check = r.Check.Name()
		v.Checks = append(v.Checks, res)
		if res.Outcome == model.Match && v.Reason == "" {
			v.Status, v.Reason = r.Status, res.Detail
		}
	}
	if v.Reason == "" {
		v.Reason = "no check matched: " + summarize(v.Checks)
	}
	return v
}

func summarize(rs []model.CheckResult) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, r.Check+" "+string(r.Outcome))
	}
	return strings.Join(parts, ", ")
}

func errResult(err error) model.CheckResult {
	return model.CheckResult{Outcome: model.Errored, Detail: err.Error()}
}

// Missing matches when git reports the worktree folder as gone.
type Missing struct{}

// Name implements Check.
func (Missing) Name() string { return "missing" }

// Eval implements Check.
func (Missing) Eval(_ context.Context, s *Subject) model.CheckResult {
	if s.WT.Prunable {
		return model.CheckResult{Outcome: model.Match, Detail: "folder is gone: " + s.WT.PruneReason}
	}
	return model.CheckResult{Outcome: model.NoMatch, Detail: "folder exists"}
}

// Empty matches when the worktree's ref never moved since it was created: a fresh
// worktree whose tip is on the target only because nothing was committed yet.
// Without a reflog it is skipped, so the worktree falls through to the merge checks.
type Empty struct{}

// Name implements Check.
func (Empty) Name() string { return "empty" }

// Eval implements Check.
func (Empty) Eval(ctx context.Context, s *Subject) model.CheckResult {
	dir, ref := s.Repo.Root, "refs/heads/"+s.WT.Branch
	if s.WT.Branch == "" {
		if s.WT.Prunable {
			return model.CheckResult{Outcome: model.Skipped, Detail: "detached and folder is gone"}
		}
		dir, ref = s.WT.Path, "HEAD"
	}
	shas, err := gitx.ReflogSHAs(ctx, s.Git, dir, ref)
	if err != nil {
		return model.CheckResult{Outcome: model.Skipped, Detail: "no reflog: " + firstLine(err.Error())}
	}
	if len(shas) == 0 {
		return model.CheckResult{Outcome: model.Skipped, Detail: "no reflog"}
	}
	for _, sha := range shas {
		if sha != s.WT.Head {
			return model.CheckResult{Outcome: model.NoMatch, Detail: fmt.Sprintf("%s moved %d time(s) since creation", shortRef(ref), len(shas)-1)}
		}
	}
	return model.CheckResult{Outcome: model.Match, Detail: fmt.Sprintf("no commits of its own since it was created at %s", short(s.WT.Head))}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// Ancestor matches when the tip is reachable from a target ref: fast-forward and
// merge-commit merges, or a branch that never got commits of its own.
type Ancestor struct{}

// Name implements Check.
func (Ancestor) Name() string { return "ancestor" }

// Eval implements Check.
func (Ancestor) Eval(ctx context.Context, s *Subject) model.CheckResult {
	return eachRef(s, func(ref string) (bool, error) {
		return gitx.IsAncestor(ctx, s.Git, s.Repo.Root, s.head(), ref)
	}, "tip %s is contained in %s", "tip %s is not in %s")
}

// EmptyDiff matches when the branch adds no changes relative to its merge base,
// e.g. commits that were later reverted on the same branch.
type EmptyDiff struct{}

// Name implements Check.
func (EmptyDiff) Name() string { return "empty-diff" }

// Eval implements Check.
func (EmptyDiff) Eval(ctx context.Context, s *Subject) model.CheckResult {
	return eachRef(s, func(ref string) (bool, error) {
		return gitx.DiffEmpty(ctx, s.Git, s.Repo.Root, ref, s.head())
	}, "%s adds no changes over %s", "%s has changes not in %s")
}

// eachRef matches if test matches against any target ref. An error on one ref is
// reported only when no other ref matched.
func eachRef(s *Subject, test func(ref string) (bool, error), yes, no string) model.CheckResult {
	if s.WT.Head == "" {
		return model.CheckResult{Outcome: model.Skipped, Detail: "no HEAD"}
	}
	var firstErr error
	for _, ref := range s.Target.Refs {
		ok, err := test(ref)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			return model.CheckResult{Outcome: model.Match, Detail: fmt.Sprintf(yes, short(s.WT.Head), shortRef(ref))}
		}
	}
	if firstErr != nil {
		return errResult(firstErr)
	}
	return model.CheckResult{Outcome: model.NoMatch, Detail: fmt.Sprintf(no, short(s.WT.Head), strings.Join(shortRefs(s.Target.Refs), ", "))}
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func shortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	return strings.TrimPrefix(ref, "refs/remotes/")
}

func shortRefs(refs []string) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = shortRef(r)
	}
	return out
}
