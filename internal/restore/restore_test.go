package restore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/oplog"
	"github.com/thecloudranger/zap/internal/plan"
	"github.com/thecloudranger/zap/internal/testkit"
)

func TestZapThenUndoRoundTrip(t *testing.T) {
	repo := testkit.NewRepo(t)
	merged := testkit.AddWorktree(t, repo, ".claude/worktrees/merged", "worktree-merged")
	testkit.Commit(t, merged, "m.txt", "m\n", "merged work")
	testkit.MergeFF(t, repo, "worktree-merged")
	empty := testkit.AddWorktree(t, repo, ".claude/worktrees/empty", "worktree-empty")
	gone := testkit.AddWorktree(t, repo, ".worktrees/gone", "gone")
	testkit.Commit(t, gone, "g.txt", "g\n", "unmerged")
	require.NoError(t, os.RemoveAll(gone))
	detached := filepath.Join(testkit.TempDir(t), "codex-1")
	testkit.Git(t, repo, "worktree", "add", "-q", "--detach", detached)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".zap"), []byte(`min_age = "0"`), 0o644))

	heads := map[string]string{
		"worktree-merged": testkit.Git(t, repo, "rev-parse", "worktree-merged"),
		"worktree-empty":  testkit.Git(t, repo, "rev-parse", "worktree-empty"),
		"gone":            testkit.Git(t, repo, "rev-parse", "gone"),
	}

	log := oplog.Log{Dir: testkit.TempDir(t)}
	p, err := plan.Build(t.Context(), gitx.Exec{}, repo, plan.Options{Now: time.Now()})
	require.NoError(t, err)
	res, err := plan.Apply(t.Context(), gitx.Exec{}, p, plan.ApplyOptions{Now: time.Now, Log: log})
	require.NoError(t, err)
	require.Len(t, res, 4)
	for _, r := range res {
		require.Equal(t, plan.Zapped, r.Outcome, r.Path+": "+r.Detail)
	}
	branches := testkit.Git(t, repo, "branch")
	assert.NotContains(t, branches, "worktree-merged")
	assert.Contains(t, branches, "gone", "unmerged branch of a missing worktree was kept")

	// Dry run changes nothing.
	_, outs, err := Undo(t.Context(), gitx.Exec{}, log, Options{DryRun: true, Now: time.Now})
	require.NoError(t, err)
	assert.Len(t, outs, 4)
	assert.NoDirExists(t, merged)

	// Someone reused one path in the meantime.
	require.NoError(t, os.MkdirAll(empty, 0o755))

	run, outs, err := Undo(t.Context(), gitx.Exec{}, log, Options{Now: time.Now})
	require.NoError(t, err)
	assert.NotEmpty(t, run)
	for _, o := range outs {
		assert.Equal(t, oplog.Restored, o.Result, "%s: %s", o.Path, o.Detail)
	}

	for b, sha := range heads {
		assert.Equal(t, sha, testkit.Git(t, repo, "rev-parse", "refs/heads/"+b), "branch %s restored at its old tip", b)
	}
	assert.Equal(t, "worktree-merged", testkit.Git(t, merged, "branch", "--show-current"), "worktree re-added on its branch")
	assert.DirExists(t, detached)
	assert.NoDirExists(t, gone, "a folder that was already gone is not recreated")
	wts := testkit.Git(t, repo, "worktree", "list")
	assert.NotContains(t, wts, empty, "occupied path is left alone")

	_, _, err = Undo(t.Context(), gitx.Exec{}, log, Options{Now: time.Now})
	assert.ErrorIs(t, err, ErrNothingToUndo, "a run is undone only once")
	_, _, err = Undo(t.Context(), gitx.Exec{}, log, Options{Run: "nope", Now: time.Now})
	assert.ErrorIs(t, err, ErrUnknownRun)
}

func TestPendingPicksLatestRun(t *testing.T) {
	entries := []oplog.Entry{ // newest first, as Log.Read returns them
		{Run: "r2", Path: "/b", Result: oplog.Restored},
		{Run: "r2", Path: "/b", Result: oplog.Zapped},
		{Run: "r1", Path: "/a2", Result: oplog.Zapped},
		{Run: "r1", Path: "/x", Result: oplog.Skipped},
		{Run: "r1", Path: "/a1", Result: oplog.Zapped},
	}
	run, pending, err := Pending(entries, "")
	require.NoError(t, err)
	assert.Equal(t, "r1", run, "r2 was already undone")
	require.Len(t, pending, 2)
	assert.Equal(t, "/a1", pending[0].Path, "oldest first")

	_, _, err = Pending(entries, "r2")
	assert.ErrorIs(t, err, ErrNothingToUndo)
}
