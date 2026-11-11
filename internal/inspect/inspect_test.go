package inspect

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/testkit"
)

// TestRepoScenarios builds one repo with a worktree per scenario and checks each verdict.
func TestRepoScenarios(t *testing.T) {
	t0 := time.Now().Add(-time.Minute)
	repo := testkit.NewRepo(t)
	testkit.AddOrigin(t, repo)

	ff := testkit.AddWorktree(t, repo, ".claude/worktrees/ff", "ff")
	testkit.Commit(t, ff, "ff.txt", "ff\n", "ff work")
	testkit.MergeFF(t, repo, "ff")

	mc := testkit.AddWorktree(t, repo, ".worktrees/mc", "mc")
	testkit.Commit(t, mc, "mc.txt", "mc\n", "mc work")
	testkit.MergeCommit(t, repo, "mc")

	remote := testkit.AddWorktree(t, repo, ".worktrees/remote", "remote")
	testkit.Commit(t, remote, "remote.txt", "r\n", "remote work")
	testkit.Git(t, repo, "push", "-q", "origin", "main") // origin must contain local main first
	testkit.MergeOnRemote(t, repo, "remote")

	reverted := testkit.AddWorktree(t, repo, ".worktrees/reverted", "reverted")
	testkit.Commit(t, reverted, "tmp.txt", "x\n", "try")
	testkit.Git(t, reverted, "revert", "--no-edit", "HEAD")

	open := testkit.AddWorktree(t, repo, ".worktrees/open", "open")
	testkit.Commit(t, open, "open.txt", "o\n", "unmerged work")
	testkit.Dirty(t, open)

	gone := testkit.AddWorktree(t, repo, ".worktrees/gone", "gone")
	require.NoError(t, os.RemoveAll(gone))

	detached := testkit.TempDir(t) + "/codex-1"
	testkit.Git(t, repo, "worktree", "add", "-q", "--detach", detached)

	rep, err := Repo(t.Context(), gitx.Exec{}, open, Options{})
	require.NoError(t, err)
	assert.Equal(t, repo, rep.Root)
	assert.Equal(t, "main", rep.Target.Name)
	assert.Equal(t, []string{"refs/heads/main", "refs/remotes/origin/main"}, rep.Target.Refs)

	got := map[string]model.Assessment{}
	for _, a := range rep.Worktrees {
		got[a.Path] = a
	}
	want := map[string]model.Status{
		repo:     model.StatusMain,
		ff:       model.StatusMerged,
		mc:       model.StatusMerged,
		remote:   model.StatusMerged,
		reverted: model.StatusMerged,
		open:     model.StatusActive,
		gone:     model.StatusMissing,
		detached: model.StatusMerged, // no commits of its own: tip is on main
	}
	require.Len(t, got, len(want))
	for path, status := range want {
		a := got[path]
		assert.Equal(t, status, a.Verdict.Status, "%s: %s", path, a.Verdict.Reason)
	}

	assert.Contains(t, got[remote].Verdict.Reason, "origin/main", "merged on the remote only")
	assert.Equal(t, "empty-diff", got[reverted].Verdict.Checks[len(got[reverted].Verdict.Checks)-1].Check)

	o := got[open].Facts
	assert.True(t, o.Dirty)
	assert.Equal(t, 1, o.Changes)
	assert.False(t, got[ff].Facts.Dirty)
	assert.True(t, o.Created.After(t0), "created %v", o.Created)
	assert.True(t, o.LastActivity.After(t0), "last activity from file mtimes, not the fixed 2026-01-01 commit date")
	assert.True(t, got[gone].Facts.Created.After(t0), "created is known even when the folder is gone")
	assert.Empty(t, got[open].Errors)
}

func TestTargetOverride(t *testing.T) {
	repo := testkit.NewRepo(t)
	testkit.Git(t, repo, "branch", "develop")
	wt := testkit.AddWorktree(t, repo, ".worktrees/a", "a")
	testkit.Commit(t, wt, "a.txt", "a\n", "a")
	testkit.Git(t, repo, "-c", "advice.detachedHead=false", "checkout", "-q", "develop")
	testkit.MergeFF(t, repo, "a")

	rep, err := Repo(t.Context(), gitx.Exec{}, repo, Options{Target: "develop"})
	require.NoError(t, err)
	for _, a := range rep.Worktrees {
		if a.Path == wt {
			assert.Equal(t, model.StatusMerged, a.Verdict.Status)
		}
	}

	rep, err = Repo(t.Context(), gitx.Exec{}, repo, Options{})
	require.NoError(t, err)
	for _, a := range rep.Worktrees {
		if a.Path == wt {
			assert.Equal(t, model.StatusActive, a.Verdict.Status, "not merged into main")
		}
	}

	_, err = Repo(t.Context(), gitx.Exec{}, repo, Options{Target: "nope"})
	assert.ErrorIs(t, err, gitx.ErrNoTarget)
}
