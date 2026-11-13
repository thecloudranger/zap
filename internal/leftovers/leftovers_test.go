package leftovers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, path string, n int, mod time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", n)), 0o644))
	require.NoError(t, os.Chtimes(path, mod, mod))
}

// slug writes a path the way Claude Code names its project folders.
func slug(p string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(p)
}

func TestFind(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	api := filepath.Join(home, "dev", "my-api.v2")
	web := filepath.Join(home, "dev", "web")
	require.NoError(t, os.MkdirAll(api, 0o755))
	require.NoError(t, os.MkdirAll(web, 0o755))
	old := time.Now().Add(-72 * time.Hour)
	recent := time.Now().Add(-time.Hour)

	ct := filepath.Join(tmp, "claude-501")
	write(t, filepath.Join(ct, slug(api), "s1", "tasks", "a.output"), 300_000, old)
	write(t, filepath.Join(ct, slug(api), "s2", "scratchpad", "b.bin"), 100_000, recent)
	write(t, filepath.Join(ct, slug(web), "s3", "c.txt"), 10_000, old)
	write(t, filepath.Join(home, ".claude", "projects", slug(api), "s1.jsonl"), 50_000, old)
	write(t, filepath.Join(home, ".claude", "projects", "-gone-project", "s9.jsonl"), 5_000, old)
	write(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "r.jsonl"), 20_000, recent)

	gs, err := Find(t.Context(), Options{Home: home, TmpRoot: tmp, UID: 501})
	require.NoError(t, err)
	require.Len(t, gs, 3)

	temp := gs[0]
	assert.Equal(t, "claude", temp.Agent)
	assert.Equal(t, Temp, temp.Kind)
	assert.Equal(t, 3, temp.Sessions)
	require.Len(t, temp.Items, 2)
	assert.Equal(t, "~/dev/my-api.v2", temp.Items[0].Label, "hyphen and dot both rebuilt from the slug")
	assert.Equal(t, 2, temp.Items[0].Sessions)
	assert.WithinDuration(t, recent, temp.Items[0].LastUsed, time.Second)
	assert.Equal(t, "~/dev/web", temp.Items[1].Label)
	assert.GreaterOrEqual(t, temp.Bytes, int64(410_000))

	tr := gs[1]
	assert.Equal(t, Transcripts, tr.Kind)
	assert.Equal(t, 2, tr.Sessions)
	assert.Equal(t, "-gone-project", tr.Items[1].Label, "a project that no longer exists keeps its folder name")
	assert.Contains(t, tr.Note, "resume")

	cx := gs[2]
	assert.Equal(t, "codex", cx.Agent)
	assert.Equal(t, 1, cx.Sessions)
	assert.Empty(t, cx.Items)
	assert.Equal(t, temp.Bytes+tr.Bytes+cx.Bytes, Total(gs))
}

func TestFindNothing(t *testing.T) {
	gs, err := Find(t.Context(), Options{Home: t.TempDir(), TmpRoot: t.TempDir(), UID: 501})
	require.NoError(t, err)
	assert.Empty(t, gs)
}

func TestResolveSlugDotFolder(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	p := filepath.Join(home, ".claude", "worktrees", "fix-login")
	require.NoError(t, os.MkdirAll(p, 0o755))
	assert.Equal(t, p, resolveSlug(slug(p)))
}

func TestResolveSlugLeadingDash(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	p := filepath.Join(home, "-odd", "deep-clean.worktrees", "a1b2-c3d4")
	require.NoError(t, os.MkdirAll(p, 0o755))
	assert.Equal(t, p, resolveSlug(slug(p)))
}
