package safety

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/policy"
	"github.com/thecloudranger/zap/internal/procs"
)

func TestGates(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	ok := model.Assessment{
		Worktree: model.Worktree{Path: "/r/w", Branch: "feat", Head: "abc"},
		Facts:    model.Facts{LastActivity: old, Created: old},
	}
	tests := []struct {
		name    string
		mutate  func(a *model.Assessment, p *policy.Repo)
		pass    bool
		failing string
	}{
		{"all pass", func(*model.Assessment, *policy.Repo) {}, true, ""},
		{"main", func(a *model.Assessment, _ *policy.Repo) { a.Main = true }, false, "not-main"},
		{"target branch", func(a *model.Assessment, _ *policy.Repo) { a.Branch = "main" }, false, "unprotected"},
		{"protected glob", func(a *model.Assessment, p *policy.Repo) { a.Branch = "release/2"; p.Protect = []string{"release/*"} }, false, "unprotected"},
		{"locked", func(a *model.Assessment, _ *policy.Repo) { a.Locked, a.LockReason = true, "claude agent" }, false, "unlocked"},
		{"inspect error", func(a *model.Assessment, _ *policy.Repo) { a.Errors = []string{"status: boom"} }, false, "inspected"},
		{"dirty", func(a *model.Assessment, _ *policy.Repo) { a.Facts.Dirty, a.Facts.Changes = true, 2 }, false, "clean"},
		{"too recent", func(a *model.Assessment, _ *policy.Repo) { a.Facts.LastActivity = now.Add(-time.Hour) }, false, "old-enough"},
		{"unknown activity", func(a *model.Assessment, _ *policy.Repo) { a.Facts.LastActivity = time.Time{} }, false, "old-enough"},
		{"missing uses created", func(a *model.Assessment, _ *policy.Repo) {
			a.Prunable, a.Facts.LastActivity = true, time.Time{}
		}, true, ""},
		{"cwd inside", func(a *model.Assessment, _ *policy.Repo) { a.Path = "/r/w" }, false, "not-cwd"},
		{"detached passes protect", func(a *model.Assessment, _ *policy.Repo) { a.Branch, a.Detached = "", true }, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p := ok, policy.Defaults()
			tt.mutate(&a, &p)
			s := &Subject{A: a, Target: model.Target{Name: "main"}, Policy: p, Now: now, Cwd: "/elsewhere"}
			if tt.failing == "not-cwd" {
				s.Cwd = "/r/w/src"
			}
			res, pass := Evaluate(s, Default())
			assert.Equal(t, tt.pass, pass)
			assert.Len(t, res, len(Default()), "every gate always runs")
			for _, r := range res {
				if r.Check == tt.failing {
					assert.Equal(t, model.Fail, r.Outcome, r.Detail)
				} else {
					assert.NotEqual(t, model.Fail, r.Outcome, "%s: %s", r.Check, r.Detail)
				}
			}
		})
	}
}

func TestNotInUse(t *testing.T) {
	a := model.Assessment{Worktree: model.Worktree{Path: "/r/w"}}
	eval := func(p *Procs, prunable bool) model.CheckResult {
		a := a
		a.Prunable = prunable
		return NotInUse{}.Eval(&Subject{A: a, Procs: p})
	}
	assert.Equal(t, model.Skipped, eval(nil, false).Outcome)
	assert.Equal(t, model.Pass, eval(&Procs{List: []procs.Proc{{PID: 1, Cwd: "/r/w2"}}}, false).Outcome)
	r := eval(&Procs{List: []procs.Proc{{PID: 42, Command: "claude", Cwd: "/r/w/src"}}}, false)
	assert.Equal(t, model.Fail, r.Outcome)
	assert.Equal(t, "in use by claude (pid 42)", r.Detail)
	assert.Equal(t, model.Fail, eval(&Procs{Err: procs.ErrUnavailable}, false).Outcome, "fail closed")
	assert.Equal(t, model.Skipped, eval(&Procs{Err: procs.ErrUnavailable}, true).Outcome, "nothing to protect when the folder is gone")
}

func TestHiddenAndKeptIgnored(t *testing.T) {
	p := policy.Defaults()
	s := &Subject{Policy: p, A: model.Assessment{Facts: model.Facts{Ignored: []string{"node_modules/", ".env"}, Hidden: 2}}}
	r := NoKeptIgnored{}.Eval(s)
	assert.Equal(t, model.Fail, r.Outcome)
	assert.Equal(t, "holds ignored .env, which would be lost", r.Detail)
	assert.Equal(t, model.Fail, NoHiddenChanges{}.Eval(s).Outcome)

	s.A.Facts = model.Facts{Ignored: []string{"a.key", "b.key", "c.key", "d.key"}}
	assert.Contains(t, NoKeptIgnored{}.Eval(s).Detail, "and 1 more")
	s.A.Facts = model.Facts{Ignored: []string{"node_modules/", "dist/"}}
	assert.Equal(t, model.Pass, NoKeptIgnored{}.Eval(s).Outcome)
	assert.Equal(t, model.Pass, NoHiddenChanges{}.Eval(s).Outcome)
}

func TestStaleLocks(t *testing.T) {
	for reason, want := range map[string]int{
		"claude agent agent-1 (pid 4242)": 4242, "PID: 7": 7, "pid=99 started by codex": 99,
		"rapid 12": 0, "agent running": 0, "": 0,
	} {
		pid, ok := LockPID(reason)
		assert.Equal(t, want, pid, reason)
		assert.Equal(t, want > 0, ok, reason)
	}

	dead := func(int) bool { return false }
	alive := func(int) bool { return true }
	locked := model.Assessment{Worktree: model.Worktree{Locked: true, LockReason: "agent (pid 4242)"}}

	r := Unlocked{}.Eval(&Subject{A: locked, Alive: alive})
	assert.Equal(t, model.Fail, r.Outcome, "a live owner keeps its lock")

	r = Unlocked{}.Eval(&Subject{A: locked, Alive: dead})
	assert.Equal(t, model.Fail, r.Outcome, "stale, but not released without the flag")
	assert.Contains(t, r.Detail, "--unlock-stale")

	r = Unlocked{}.Eval(&Subject{A: locked, Alive: dead, UnlockStale: true})
	assert.Equal(t, model.Pass, r.Outcome)

	noPID := model.Assessment{Worktree: model.Worktree{Locked: true, LockReason: "on a USB drive"}}
	assert.Equal(t, model.Fail, Unlocked{}.Eval(&Subject{A: noPID, Alive: dead, UnlockStale: true}).Outcome,
		"a lock without a pid is never released")
}
