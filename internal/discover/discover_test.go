package discover

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/testkit"
)

func TestFind(t *testing.T) {
	base := testkit.TempDir(t)
	dev := filepath.Join(base, "dev")
	agents := filepath.Join(base, "agents")
	elsewhere := filepath.Join(base, "elsewhere")

	mk := func(path string) string {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		testkit.Git(t, filepath.Dir(path), "init", "-q", "-b", "main", filepath.Base(path))
		testkit.Commit(t, path, "README.md", "x\n", "init")
		return path
	}
	app := mk(filepath.Join(dev, "work", "app"))
	testkit.AddWorktree(t, app, ".claude/worktrees/a", "a") // same repo: must not be listed twice
	nested := mk(filepath.Join(app, "sandbox", "nested"))
	named := mk(filepath.Join(dev, "build")) // a project named like a build folder
	mk(filepath.Join(dev, "web", "node_modules", "pkg"))
	deep := mk(filepath.Join(dev, "a", "b", "c", "d", "e", "f", "g", "too-deep"))

	// A worktree in an agent folder whose repository lives outside every root.
	outside := mk(filepath.Join(elsewhere, "lib"))
	testkit.Git(t, outside, "worktree", "add", "-q", "-b", "agent", filepath.Join(agents, "codex", "1", "lib"))

	// A worktree whose repository was deleted.
	gone := mk(filepath.Join(elsewhere, "gone"))
	testkit.Git(t, gone, "worktree", "add", "-q", "-b", "x", filepath.Join(agents, "orphan"))
	require.NoError(t, os.RemoveAll(gone))

	// A symlink loop must not hang the walk.
	require.NoError(t, os.Symlink(dev, filepath.Join(dev, "loop")))

	res, err := Find(t.Context(), []string{dev, agents, filepath.Join(dev, "work"), filepath.Join(base, "missing")}, Options{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{app, nested, named, outside}, res.Repos)
	assert.NotContains(t, res.Repos, deep, "beyond max depth")
	assert.Positive(t, res.Visited)

	res, err = Find(t.Context(), []string{dev}, Options{MaxDepth: 12})
	require.NoError(t, err)
	assert.Contains(t, res.Repos, deep)
}

func TestFindUnreadableDirIsAWarning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := testkit.TempDir(t)
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	res, err := Find(t.Context(), []string{root}, Options{})
	require.NoError(t, err)
	require.Len(t, res.Warnings, 1)
	assert.True(t, strings.Contains(res.Warnings[0], "locked"), res.Warnings[0])
}

func TestFindCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Find(ctx, []string{testkit.TempDir(t)}, Options{})
	assert.ErrorIs(t, err, context.Canceled)
}
