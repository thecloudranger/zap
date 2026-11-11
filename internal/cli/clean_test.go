package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/plan"
	"github.com/thecloudranger/zap/internal/policy"
	"github.com/thecloudranger/zap/internal/testkit"
	"github.com/thecloudranger/zap/internal/tui"
)

// session runs zap in-process as if on a terminal, with typed input.
type session struct {
	t     *testing.T
	home  string
	repo  string
	stdin string
}

func (s session) run(args ...string) (string, int) {
	s.t.Helper()
	var out bytes.Buffer
	d := Deps{
		Git:       gitx.Exec{},
		In:        strings.NewReader(s.stdin),
		Out:       &out,
		Err:       &out,
		IsTTY:     func() bool { return true },
		Getwd:     func() (string, error) { return s.repo, nil },
		Now:       time.Now,
		Home:      s.home,
		ConfigDir: filepath.Join(s.home, "config"),
		StateDir:  filepath.Join(s.home, "state"),
	}
	root := NewRoot(d)
	root.SetArgs(args)
	code := Exit(root.ExecuteContext(s.t.Context()), &out)
	return out.String(), code
}

// mergedRepo returns an opted-in repo with one merged worktree old enough to zap.
func mergedRepo(t *testing.T) (repo, wt string) {
	t.Helper()
	repo = testkit.NewRepo(t)
	wt = testkit.AddWorktree(t, repo, ".claude/worktrees/done", "worktree-done")
	testkit.Commit(t, wt, "d.txt", "d\n", "done")
	testkit.MergeFF(t, repo, "worktree-done")
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".zap"), []byte(`min_age = "0"`), 0o644))
	return repo, wt
}

func TestCleanDisclaimerAndConfirm(t *testing.T) {
	repo, wt := mergedRepo(t)
	home := testkit.TempDir(t)

	out, code := session{t, home, repo, "no thanks\n"}.run("clean")
	assert.Equal(t, ExitRefused, code)
	assert.Contains(t, out, `Type "zap" to accept`)
	assert.Contains(t, out, "cannot be restored", "disclaimer warns about ignored files")
	assert.DirExists(t, wt)
	cfg, err := policy.LoadConfig(filepath.Join(home, "config"))
	require.NoError(t, err)
	assert.False(t, cfg.Accepted())

	out, code = session{t, home, repo, "zap\nn\n"}.run("clean")
	assert.Equal(t, ExitRefused, code, out)
	assert.Contains(t, out, "Zap 1 worktree? [y/N]")
	assert.DirExists(t, wt, "answering n changes nothing")
	cfg, err = policy.LoadConfig(filepath.Join(home, "config"))
	require.NoError(t, err)
	assert.True(t, cfg.Accepted(), "acceptance is remembered even if the run is cancelled")

	out, code = session{t, home, repo, "y\n"}.run("clean")
	assert.Equal(t, ExitOK, code, out)
	assert.NotContains(t, out, "Type \"zap\"", "disclaimer is asked only once")
	assert.Contains(t, out, "✓ zapped")
	assert.Contains(t, out, "Zapped 1 worktree.")
	assert.Regexp(t, `Freed about \S+ \S+  ·  free space`, out)
	assert.NoDirExists(t, wt)

	out, code = session{t, home, repo, ""}.run("history")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, out, "zapped")
	assert.Contains(t, out, "worktree-done")
	assert.Contains(t, out, "refs/zap/backup/")
}

func TestCleanEOFIsNo(t *testing.T) {
	repo, wt := mergedRepo(t)
	_, code := session{t, testkit.TempDir(t), repo, ""}.run("clean")
	assert.Equal(t, ExitRefused, code)
	assert.DirExists(t, wt)
}

func TestInit(t *testing.T) {
	repo := testkit.NewRepo(t)
	wt := testkit.AddWorktree(t, repo, ".worktrees/a", "a")
	s := session{t, testkit.TempDir(t), wt, ""}

	out, code := s.run("init")
	assert.Equal(t, ExitOK, code, out)
	data, err := os.ReadFile(filepath.Join(repo, ".zap"))
	require.NoError(t, err, "written at the repo root even when run from a linked worktree")
	assert.Equal(t, policy.Template, string(data))

	out, code = s.run("init")
	assert.Equal(t, ExitFailure, code)
	assert.Contains(t, out, "already opted in")
}

func TestKeepUnselected(t *testing.T) {
	plans := []plan.Plan{{Items: []plan.Item{
		{Path: "/a", Action: plan.Remove},
		{Path: "/b", Action: plan.Prune},
		{Path: "/c", Action: plan.Keep, Refusal: "locked"},
	}}}
	got := keepUnselected(plans, map[tui.Key]bool{{Plan: 0, Item: 1}: true, {Plan: 0, Item: 2}: true})
	assert.Equal(t, plan.Keep, got[0].Items[0].Action, "unselected removal is kept")
	assert.Equal(t, "not selected", got[0].Items[0].Refusal)
	assert.Equal(t, plan.Prune, got[0].Items[1].Action, "selected stays")
	assert.Equal(t, plan.Keep, got[0].Items[2].Action, "a kept item can never be turned into a removal")
	assert.Equal(t, "locked", got[0].Items[2].Refusal)
	assert.Equal(t, plan.Remove, plans[0].Items[0].Action, "input plans are not mutated")
}
