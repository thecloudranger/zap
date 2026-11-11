// Package safety holds the gates every candidate must pass before zap removes it.
// Gates are independent and all run, so a refusal lists every reason at once.
package safety

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/policy"
	"github.com/thecloudranger/zap/internal/procs"
)

// Subject is what a gate looks at.
type Subject struct {
	A      model.Assessment
	Target model.Target
	Policy policy.Repo
	Now    time.Time
	Cwd    string // zap's working directory; a worktree containing it is kept
	Procs  *Procs // running processes; nil means not checked
	// UnlockStale lets a lock whose recorded pid is no longer running pass Unlocked.
	UnlockStale bool
	// Alive reports whether a pid is running; nil means procs.Alive.
	Alive func(pid int) bool
}

// Procs is a snapshot of running processes' working directories.
type Procs struct {
	List []procs.Proc
	Err  error // the snapshot could not be taken
}

// Gate is one safety rule.
type Gate interface {
	Name() string
	Eval(s *Subject) model.CheckResult
}

// Default is the gate list used by clean.
func Default() []Gate {
	return []Gate{NotMain{}, Unprotected{}, Unlocked{}, Inspected{}, Clean{}, NoHiddenChanges{},
		NoKeptIgnored{}, OldEnough{}, NotCwd{}, NotInUse{}}
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

// Unlocked refuses locked worktrees; agents lock the worktrees they are using. A lock
// whose reason names a pid that is no longer running is stale: it passes only when the
// user asked for --unlock-stale.
type Unlocked struct{}

// Name implements Gate.
func (Unlocked) Name() string { return "unlocked" }

// Eval implements Gate.
func (Unlocked) Eval(s *Subject) model.CheckResult {
	if !s.A.Locked {
		return pass("not locked")
	}
	what := "locked"
	if s.A.LockReason != "" {
		what += ": " + s.A.LockReason
	}
	if !StaleLock(s.A.LockReason, s.Alive) {
		return fail("%s", what)
	}
	pid, _ := LockPID(s.A.LockReason)
	if s.UnlockStale {
		return pass("stale lock (pid %d is not running); it will be released", pid)
	}
	return fail("%s (pid %d is not running; --unlock-stale releases it)", what, pid)
}

var pidPattern = regexp.MustCompile(`(?i)\bpid[\s:=#]*(\d+)`)

// LockPID extracts the pid a lock reason names, such as "agent-1 (pid 4242)".
func LockPID(reason string) (int, bool) {
	m := pidPattern.FindStringSubmatch(reason)
	if m == nil {
		return 0, false
	}
	pid, err := strconv.Atoi(m[1])
	return pid, err == nil && pid > 0
}

// StaleLock reports whether a lock reason names a pid that is no longer running.
func StaleLock(reason string, alive func(int) bool) bool {
	pid, ok := LockPID(reason)
	if !ok {
		return false
	}
	if alive == nil {
		alive = procs.Alive
	}
	return !alive(pid)
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

// NotInUse refuses a worktree that another process (a shell, editor or agent) is
// working in. If processes cannot be listed, it refuses: doubt means keep.
type NotInUse struct{}

// Name implements Gate.
func (NotInUse) Name() string { return "not-in-use" }

// Eval implements Gate.
func (NotInUse) Eval(s *Subject) model.CheckResult {
	switch {
	case s.Procs == nil:
		return model.CheckResult{Outcome: model.Skipped, Detail: "processes not checked"}
	case s.A.Prunable:
		return model.CheckResult{Outcome: model.Skipped, Detail: "folder is gone"}
	case s.Procs.Err != nil:
		return fail("cannot check for processes using it: %v", s.Procs.Err)
	}
	in := procs.Inside(s.Procs.List, s.A.Path)
	if len(in) == 0 {
		return pass("no process is working in it")
	}
	names := make([]string, 0, len(in))
	for _, p := range in {
		names = append(names, fmt.Sprintf("%s (pid %d)", p.Command, p.PID))
	}
	return fail("in use by %s", strings.Join(names, ", "))
}

// NoHiddenChanges refuses a worktree with files git status is told not to show
// (assume-unchanged, or skip-worktree outside a sparse checkout): "clean" can't be trusted.
type NoHiddenChanges struct{}

// Name implements Gate.
func (NoHiddenChanges) Name() string { return "no-hidden" }

// Eval implements Gate.
func (NoHiddenChanges) Eval(s *Subject) model.CheckResult {
	if s.A.Prunable {
		return model.CheckResult{Outcome: model.Skipped, Detail: "folder is gone"}
	}
	if n := s.A.Facts.Hidden; n > 0 {
		return fail("%d file(s) marked assume-unchanged or skip-worktree; git status hides their changes", n)
	}
	return pass("no files hidden from git status")
}

// NoKeptIgnored refuses a worktree holding ignored files that match keep_ignored, such
// as .env or keys: they are not in git, so removing the worktree would lose them.
type NoKeptIgnored struct{}

// Name implements Gate.
func (NoKeptIgnored) Name() string { return "keep-ignored" }

// Eval implements Gate.
func (NoKeptIgnored) Eval(s *Subject) model.CheckResult {
	if s.A.Prunable {
		return model.CheckResult{Outcome: model.Skipped, Detail: "folder is gone"}
	}
	hit := s.Policy.KeptIgnored(s.A.Facts.Ignored)
	if len(hit) == 0 {
		return pass("no ignored files matching keep_ignored")
	}
	shown := hit
	if len(shown) > 3 {
		shown = append(shown[:3:3], fmt.Sprintf("and %d more", len(hit)-3))
	}
	return fail("holds ignored %s, which would be lost", strings.Join(shown, ", "))
}
