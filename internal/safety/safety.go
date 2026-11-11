// Package safety holds the gates every candidate must pass before zap removes it.
// Gates are independent and all run, so a refusal lists every reason at once.
package safety

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/policy"
)

// Subject is what a gate looks at.
type Subject struct {
	A      model.Assessment
	Target model.Target
	Policy policy.Repo
	Now    time.Time
	Cwd    string // zap's working directory; a worktree containing it is kept
}

// Gate is one safety rule.
type Gate interface {
	Name() string
	Eval(s *Subject) model.CheckResult
}

// Default is the gate list used by clean.
func Default() []Gate {
	return []Gate{NotMain{}, Unprotected{}, Unlocked{}, Inspected{}, Clean{}, OldEnough{}, NotCwd{}}
}

// Evaluate runs every gate. ok is true only if all pass.
func Evaluate(s *Subject, gates []Gate) (results []model.CheckResult, ok bool) {
	ok = true
	for _, g := range gates {
		r := g.Eval(s)
		r.Check = g.Name()
		if r.Outcome != model.Pass && r.Outcome != model.Skipped {
			ok = false
		}
		results = append(results, r)
	}
	return results, ok
}

func pass(format string, a ...any) model.CheckResult {
	return model.CheckResult{Outcome: model.Pass, Detail: fmt.Sprintf(format, a...)}
}

func fail(format string, a ...any) model.CheckResult {
	return model.CheckResult{Outcome: model.Fail, Detail: fmt.Sprintf(format, a...)}
}

// NotMain refuses the repository's main worktree.
type NotMain struct{}

// Name implements Gate.
func (NotMain) Name() string { return "not-main" }

// Eval implements Gate.
func (NotMain) Eval(s *Subject) model.CheckResult {
	if s.A.Main || s.A.Bare {
		return fail("main worktree of the repository")
	}
	return pass("linked worktree")
}

// Unprotected refuses the target branch and branches matching .zap protect patterns.
type Unprotected struct{}

// Name implements Gate.
func (Unprotected) Name() string { return "unprotected" }

// Eval implements Gate.
func (Unprotected) Eval(s *Subject) model.CheckResult {
	b := s.A.Branch
	switch {
	case b == "":
		return pass("detached HEAD, no branch")
	case b == s.Target.Name:
		return fail("branch %q is the target branch", b)
	case s.Policy.Protected(b):
		return fail("branch %q matches protect %v", b, s.Policy.Protect)
	}
	return pass("branch %q is not protected", b)
}

// Unlocked refuses locked worktrees; agents such as Claude Code lock while running.
type Unlocked struct{}

// Name implements Gate.
func (Unlocked) Name() string { return "unlocked" }

// Eval implements Gate.
func (Unlocked) Eval(s *Subject) model.CheckResult {
	if s.A.Locked {
		if s.A.LockReason != "" {
			return fail("locked: %s", s.A.LockReason)
		}
		return fail("locked")
	}
	return pass("not locked")
}

// Inspected refuses worktrees zap could not fully inspect: doubt means keep.
type Inspected struct{}

// Name implements Gate.
func (Inspected) Name() string { return "inspected" }

// Eval implements Gate.
func (Inspected) Eval(s *Subject) model.CheckResult {
	if len(s.A.Errors) > 0 {
		return fail("could not inspect: %s", s.A.Errors[0])
	}
	return pass("fully inspected")
}

// Clean refuses worktrees with modified, staged or untracked files.
type Clean struct{}

// Name implements Gate.
func (Clean) Name() string { return "clean" }

// Eval implements Gate.
func (Clean) Eval(s *Subject) model.CheckResult {
	if s.A.Prunable {
		return model.CheckResult{Outcome: model.Skipped, Detail: "folder is gone"}
	}
	if s.A.Facts.Dirty {
		return fail("%d uncommitted change(s)", s.A.Facts.Changes)
	}
	return pass("no uncommitted changes")
}

// OldEnough refuses worktrees active more recently than min_age.
type OldEnough struct{}

// Name implements Gate.
func (OldEnough) Name() string { return "old-enough" }

// Eval implements Gate.
func (OldEnough) Eval(s *Subject) model.CheckResult {
	minAge := time.Duration(s.Policy.MinAge)
	last := s.A.Facts.LastActivity
	if s.A.Prunable {
		last = s.A.Facts.Created // a missing folder has no activity of its own
	}
	if last.IsZero() {
		return fail("last activity unknown")
	}
	if age := s.Now.Sub(last); age < minAge {
		when := "just now"
		if age >= time.Minute {
			when = policy.FormatDuration(age.Truncate(time.Minute)) + " ago"
		}
		return fail("active %s, min_age is %s", when, policy.FormatDuration(minAge))
	}
	return pass("last active %s", last.Format(time.DateTime))
}

// NotCwd refuses the worktree zap is being run from.
type NotCwd struct{}

// Name implements Gate.
func (NotCwd) Name() string { return "not-cwd" }

// Eval implements Gate.
func (NotCwd) Eval(s *Subject) model.CheckResult {
	if s.Cwd == "" {
		return model.CheckResult{Outcome: model.Skipped, Detail: "working directory unknown"}
	}
	rel, err := filepath.Rel(s.A.Path, s.Cwd)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail("zap is running inside it")
	}
	return pass("zap is not running inside it")
}
