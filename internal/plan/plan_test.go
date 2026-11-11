package plan

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/oplog"
	"github.com/thecloudranger/zap/internal/testkit"
)

type fixture struct {
	repo                                                    string
	merged, empty, dirty, locked, goneMerged, goneOpen, wip string
}

// newFixture builds a repo with one worktree per planning case.
func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{repo: testkit.NewRepo(t)}
	r := f.repo
	f.merged = mergedWT(t, r, ".claude/worktrees/merged", "worktree-merged")
	f.empty = testkit.AddWorktree(t, r, ".claude/worktrees/empty", "worktree-empty")
	f.dirty = mergedWT(t, r, ".worktrees/dirty", "dirty")
	testkit.Dirty(t, f.dirty)
	f.locked = mergedWT(t, r, ".worktrees/locked", "locked")
	testkit.Git(t, r, "worktree", "lock", "--reason", "claude agent running", f.locked)
	f.goneMerged = mergedWT(t, r, ".worktrees/gone-merged", "gone-merged")
	require.NoError(t, os.RemoveAll(f.goneMerged))
	f.goneOpen = testkit.AddWorktree(t, r, ".worktrees/gone-open", "gone-open")
	testkit.Commit(t, f.goneOpen, "o.txt", "o\n", "unmerged")
	require.NoError(t, os.RemoveAll(f.goneOpen))
	f.wip = testkit.AddWorktree(t, r, ".worktrees/wip", "wip")
	testkit.Commit(t, f.wip, "w.txt", "w\n", "wip")
	return f
}

func mergedWT(t *testing.T, repo, rel, branch string) string {
	t.Helper()
	wt := testkit.AddWorktree(t, repo, rel, branch)
	testkit.Commit(t, wt, branch+".txt", branch+"\n", branch)
	testkit.MergeFF(t, repo, branch)
	return wt
}

func optIn(t *testing.T, repo, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".zap"), []byte(content), 0o644))
}

func byPath(p Plan) map[string]Item {
	m := map[string]Item{}
	for _, it := range p.Items {
		m[it.Path] = it
	}
	return m
}

func TestBuildNotOptedIn(t *testing.T) {
	f := newFixture(t)
	p, err := Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now()})
	require.NoError(t, err)
	assert.False(t, p.OptedIn)
	require.NotEmpty(t, p.Items)
	for _, it := range p.Items {
		assert.Equal(t, Keep, it.Action, it.Path)
		assert.NotEmpty(t, it.Refusal, it.Path)
	}
	optIn(t, f.repo, "") // would-be candidates name the missing file once gates pass
	require.NoError(t, os.Remove(filepath.Join(f.repo, ".zap")))
	p, err = Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now().Add(48 * time.Hour)})
	require.NoError(t, err)
	assert.Contains(t, byPath(p)[f.merged].Refusal, "zap init")
	_, err = Apply(t.Context(), gitx.Exec{}, p, ApplyOptions{Now: time.Now, Log: oplog.Log{Dir: t.TempDir()}})
	assert.Error(t, err, "apply refuses a repo without .zap")
}

func TestBuildAndApply(t *testing.T) {
	f := newFixture(t)
	optIn(t, f.repo, `min_age = "0"`)

	p, err := Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now(), Cwd: f.repo})
	require.NoError(t, err)
	assert.True(t, p.OptedIn)
	assert.Equal(t, 1, p.ActiveCount, "wip is not a candidate")

	items := byPath(p)
	want := map[string]struct {
		action       Action
		deleteBranch bool
	}{
		f.merged:     {Remove, true},
		f.empty:      {Remove, true},
		f.dirty:      {Keep, true},
		f.locked:     {Keep, true},
		f.goneMerged: {Prune, true},
		f.goneOpen:   {Prune, false},
	}
	require.Len(t, items, len(want))
	for path, w := range want {
		it := items[path]
		assert.Equal(t, w.action, it.Action, "%s: %s", path, it.Refusal)
		assert.Equal(t, w.deleteBranch, it.DeleteBranch, "%s: %s", path, it.BranchNote)
	}
	assert.Contains(t, items[f.dirty].Refusal, "uncommitted")
	assert.Contains(t, items[f.locked].Refusal, "claude agent running")
	assert.Contains(t, items[f.goneOpen].BranchNote, "commits not in main")

	logDir := t.TempDir()
	results, err := Apply(t.Context(), gitx.Exec{}, p, ApplyOptions{Now: time.Now, Cwd: f.repo, Log: oplog.Log{Dir: logDir}})
	require.NoError(t, err)
	require.Len(t, results, 4)
	for _, r := range results {
		assert.Equal(t, Zapped, r.Outcome, "%s: %s", r.Path, r.Detail)
		assert.NotEmpty(t, r.BackupRef)
	}

	rep, err := gitx.ListRepo(t.Context(), gitx.Exec{}, f.repo)
	require.NoError(t, err)
	left := map[string]bool{}
	for _, w := range rep.Worktrees {
		left[w.Path] = true
	}
	for _, gone := range []string{f.merged, f.empty, f.goneMerged, f.goneOpen} {
		assert.False(t, left[gone], "%s should be unregistered", gone)
		assert.NoDirExists(t, gone)
	}
	for _, kept := range []string{f.dirty, f.locked, f.wip} {
		assert.True(t, left[kept], "%s must be kept", kept)
		assert.DirExists(t, kept)
	}

	branches := testkit.Git(t, f.repo, "branch", "--format=%(refname:short)")
	assert.NotContains(t, branches, "worktree-merged")
	assert.NotContains(t, branches, "gone-merged")
	assert.Contains(t, branches, "gone-open", "unmerged branch of a missing worktree is kept")

	backups := testkit.Git(t, f.repo, "for-each-ref", "--format=%(refname) %(objectname)", BackupPrefix)
	assert.Contains(t, backups, "/worktree-merged "+items[f.merged].Head)
	assert.Contains(t, backups, "/gone-open "+items[f.goneOpen].Head)

	entries, _, err := oplog.Log{Dir: logDir}.Read()
	require.NoError(t, err)
	assert.Len(t, entries, 4)
}

func TestApplySkipsWhatChanged(t *testing.T) {
	f := newFixture(t)
	optIn(t, f.repo, `min_age = "0"`)
	p, err := Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now()})
	require.NoError(t, err)

	// An agent commits in the empty worktree after the plan was made.
	testkit.Commit(t, f.empty, "late.txt", "late\n", "late work")
	// Someone starts editing the merged one.
	testkit.Dirty(t, f.merged)

	results, err := Apply(t.Context(), gitx.Exec{}, p, ApplyOptions{Now: time.Now, Log: oplog.Log{Dir: t.TempDir()}})
	require.NoError(t, err)
	got := map[string]Result{}
	for _, r := range results {
		got[r.Path] = r
	}
	assert.Equal(t, Skipped, got[f.empty].Outcome)
	assert.Contains(t, got[f.empty].Detail, "HEAD moved")
	assert.Equal(t, Skipped, got[f.merged].Outcome)
	assert.DirExists(t, f.empty)
	assert.DirExists(t, f.merged)
	assert.Contains(t, testkit.Git(t, f.repo, "branch"), "worktree-empty")
}

func TestDefaultMinAgeKeepsFreshWorktrees(t *testing.T) {
	f := newFixture(t)
	optIn(t, f.repo, "")
	p, err := Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now()})
	require.NoError(t, err)
	for _, it := range p.Items {
		assert.Equal(t, Keep, it.Action, it.Path)
		if it.Status != model.StatusMissing && it.Path != f.dirty && it.Path != f.locked {
			assert.Contains(t, it.Refusal, "min_age is 1d", it.Path)
		}
	}
	zap, keep := p.Counts()
	assert.Zero(t, zap)
	assert.Equal(t, len(p.Items), keep)
}

func TestWorktreeInUseIsKept(t *testing.T) {
	f := newFixture(t)
	optIn(t, f.repo, `min_age = "0"`)
	// Something else (not zap) is working inside the merged worktree.
	pid := testkit.Git(t, f.merged, "-c", "alias.bg=!sleep 30 >/dev/null 2>&1 & echo $!", "bg")
	t.Cleanup(func() { _ = exec.Command("kill", pid).Run() })

	p, err := Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now()})
	require.NoError(t, err)
	it := byPath(p)[f.merged]
	assert.Equal(t, Keep, it.Action)
	assert.Contains(t, it.Refusal, "in use by sleep (pid "+pid+")")
	assert.Equal(t, Remove, byPath(p)[f.empty].Action, "other worktrees are unaffected")
}
