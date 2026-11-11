package sizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, path string, n int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i) // non-zero data, so filesystems cannot store it sparsely
	}
	require.NoError(t, os.WriteFile(path, b, 0o644))
}

func TestSize(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "main.go"), 100_000)
	write(t, filepath.Join(root, "node_modules", "pkg", "index.js"), 300_000)
	write(t, filepath.Join(root, "web", ".next", "cache.bin"), 200_000)
	require.NoError(t, os.Symlink("/", filepath.Join(root, "link-to-root"))) // must not be followed

	u, err := NewCounter().Size(t.Context(), root)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, u.Deps, int64(500_000), "node_modules and .next are deps")
	assert.GreaterOrEqual(t, u.Total-u.Deps, int64(100_000))
	assert.Less(t, u.Total, int64(5_000_000), "symlink was not followed")
}

func TestHardlinksCountOnce(t *testing.T) {
	store := t.TempDir()
	a, b := t.TempDir(), t.TempDir()
	write(t, filepath.Join(store, "big.bin"), 1_000_000)
	require.NoError(t, os.MkdirAll(filepath.Join(a, "node_modules"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(b, "node_modules"), 0o755))
	require.NoError(t, os.Link(filepath.Join(store, "big.bin"), filepath.Join(a, "node_modules", "big.bin")))
	require.NoError(t, os.Link(filepath.Join(store, "big.bin"), filepath.Join(b, "node_modules", "big.bin")))

	c := NewCounter()
	ua, err := c.Size(t.Context(), a)
	require.NoError(t, err)
	ub, err := c.Size(t.Context(), b)
	require.NoError(t, err)
	assert.Less(t, ua.Total, int64(100_000), "hard-linked data is not freed by removing one link")
	assert.GreaterOrEqual(t, ua.Shared, int64(1_000_000))
	assert.Equal(t, ua.Shared, ub.Shared, "each worktree reports its own shared data, independent of order")
}

func TestInDeps(t *testing.T) {
	for p, want := range map[string]bool{
		"/r/node_modules":              true,
		"/r/node_modules/a/b.js":       true,
		"/r/packages/x/node_modules/y": true,
		"/r/src/build.go":              false,
		"/r/src":                       false,
		"/r/target/debug/app":          true,
	} {
		assert.Equal(t, want, inDeps("/r", p), p)
	}
}

func TestFree(t *testing.T) {
	n, err := Free(t.TempDir())
	require.NoError(t, err)
	assert.Positive(t, n)
	_, err = Free("/nonexistent/zap")
	assert.ErrorIs(t, err, ErrNoStatfs)
}
