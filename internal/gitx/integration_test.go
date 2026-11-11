package gitx

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/testkit"
)

func TestListRepoRealGit(t *testing.T) {
	repo := testkit.NewRepo(t)
	wt := testkit.AddWorktree(t, repo, ".claude/worktrees/feat-a", "worktree-feat-a")
	gone := testkit.AddWorktree(t, repo, ".worktrees/old", "old")
	testkit.Git(t, repo, "worktree", "lock", "--reason", "agent busy", wt)
	require.NoError(t, os.RemoveAll(gone))

	// Run from inside the linked worktree: the repo is still resolved from its main root.
	got, err := ListRepo(t.Context(), Exec{}, wt)
	require.NoError(t, err)

	assert.Equal(t, repo, got.Root)
	require.Len(t, got.Worktrees, 3)
	assert.True(t, got.Worktrees[0].Main)
	assert.Equal(t, "main", got.Worktrees[0].Branch)

	byPath := map[string]int{}
	for i, w := range got.Worktrees {
		byPath[w.Path] = i
	}
	a := got.Worktrees[byPath[wt]]
	assert.Equal(t, "worktree-feat-a", a.Branch)
	assert.True(t, a.Locked)
	assert.Equal(t, "agent busy", a.LockReason)
	assert.True(t, got.Worktrees[byPath[gone]].Prunable)
}

func TestListRepoNotARepo(t *testing.T) {
	_, err := ListRepo(t.Context(), Exec{}, testkit.TempDir(t))
	assert.True(t, errors.Is(err, ErrNotRepo), "got %v", err)
}
