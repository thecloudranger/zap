package inspect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/testkit"
)

// TestGitHubStyleMerges covers how pull requests usually land: main has moved on, and the
// branch is squashed or rebased, so its own commits never reach main.
func TestGitHubStyleMerges(t *testing.T) {
	repo := testkit.NewRepo(t)
	testkit.AddOrigin(t, repo)
	wt := func(name string, files ...string) string {
		p := testkit.AddWorktree(t, repo, ".claude/worktrees/"+name, name)
		for _, f := range files {
			testkit.Commit(t, p, f, name+" "+f+"\n", name+": "+f)
		}
		return p
	}

	squashed := wt("squashed", "s1.txt", "s2.txt")
	rebased := wt("rebased", "r1.txt", "r2.txt")
	partial := wt("partial", "p1.txt")
	conflict := wt("conflict", "README.md")
	gone := wt("gone", "g.txt")
	testkit.PushTracking(t, repo, "gone")

	// main moves on, then the PRs land.
	testkit.Commit(t, repo, "other.txt", "other\n", "someone else's work")
	testkit.SquashMerge(t, repo, "squashed")
	testkit.RebaseMerge(t, repo, "rebased")
	testkit.SquashMerge(t, repo, "partial")
	testkit.Commit(t, partial, "p2.txt", "more\n", "partial: follow-up never merged")
	testkit.Commit(t, repo, "README.md", "main rewrote this\n", "conflicting change on main")
	testkit.DeleteRemoteBranch(t, repo, "gone")

	rep, err := Repo(t.Context(), gitx.Exec{}, repo, Options{AllRules: true})
	require.NoError(t, err)
	got := map[string]model.Assessment{}
	for _, a := range rep.Worktrees {
		got[a.Path] = a
	}
	want := map[string]struct {
		status model.Status
		by     string
	}{
		squashed: {model.StatusMerged, "squash"},
		rebased:  {model.StatusMerged, "cherry"},
		partial:  {model.StatusActive, ""},
		conflict: {model.StatusActive, ""},
		gone:     {model.StatusGone, "gone"},
	}
	for path, w := range want {
		a := got[path]
		assert.Equal(t, w.status, a.Verdict.Status, "%s: %s", path, a.Verdict.Reason)
		if w.by != "" {
			assert.Equal(t, w.by, decidedBy(a.Verdict), path)
		}
	}
}

// decidedBy returns the first matching check, which is the one that set the status.
func decidedBy(v model.Verdict) string {
	for _, c := range v.Checks {
		if c.Outcome == model.Match {
			return c.Check
		}
	}
	return ""
}

// TestFetch: a PR merged by someone else is only seen after fetching; a broken remote
// degrades to a warning instead of failing the scan.
func TestFetch(t *testing.T) {
	repo := testkit.NewRepo(t)
	bare := testkit.AddOrigin(t, repo)
	wt := testkit.AddWorktree(t, repo, ".claude/worktrees/pr", "pr")
	testkit.Commit(t, wt, "pr.txt", "pr\n", "pr work")
	testkit.PushTracking(t, repo, "pr")

	// A teammate merges the PR from their own clone.
	other := testkit.TempDir(t) + "/other"
	testkit.Git(t, testkit.TempDir(t), "clone", "-q", bare, other)
	testkit.Git(t, other, "merge", "-q", "--ff-only", "origin/pr")
	testkit.Git(t, other, "push", "-q", "origin", "main")

	status := func(fetch bool) (model.Status, []string) {
		rep, err := Repo(t.Context(), gitx.Exec{}, repo, Options{Fetch: fetch})
		require.NoError(t, err)
		for _, a := range rep.Worktrees {
			if a.Path == wt {
				return a.Verdict.Status, rep.Warnings
			}
		}
		t.Fatal("worktree not found")
		return "", nil
	}
	s, _ := status(false)
	assert.Equal(t, model.StatusActive, s, "not visible before fetching")
	s, warn := status(true)
	assert.Equal(t, model.StatusMerged, s)
	assert.Empty(t, warn)

	testkit.Git(t, repo, "remote", "set-url", "origin", "/nonexistent/zap-remote.git")
	s, warn = status(true)
	assert.Equal(t, model.StatusMerged, s, "last fetched state is still used")
	require.Len(t, warn, 1)
	assert.Contains(t, warn[0], "fetch failed")
}
