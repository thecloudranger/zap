package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/cli"
	"github.com/thecloudranger/zap/internal/testkit"
)

var zapBin string

// TestMain builds the real binary once; every test drives it like a user would.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "zap-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	zapBin = filepath.Join(dir, "zap")
	build := exec.Command("go", "build", "-o", zapBin, ".")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build zap:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, dir string, args ...string) result {
	t.Helper()
	cmd := exec.Command(zapBin, args...)
	cmd.Dir = dir
	cmd.Env = testkit.Env(testkit.TempDir(t))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run zap: %v", err)
	}
	return result{out.String(), errb.String(), code}
}

func TestScanTable(t *testing.T) {
	repo := testkit.NewRepo(t)
	testkit.AddWorktree(t, repo, ".claude/worktrees/feat-a", "worktree-feat-a")
	testkit.AddWorktree(t, repo, ".worktrees/fix-b", "fix-b")

	r := run(t, repo, "scan")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "2 linked worktrees")
	assert.Regexp(t, `WORKTREE\s+BRANCH\s+STATUS\s+AGE\s+ACTIVE\s+NOTES`, r.stdout)
	assert.Contains(t, r.stdout, ".claude/worktrees/feat-a")
	assert.Contains(t, r.stdout, "worktree-feat-a")
	assert.Contains(t, r.stdout, "fix-b")
	assert.Empty(t, r.stderr)
}

func TestScanJSONFromInsideWorktreeAndByPath(t *testing.T) {
	repo := testkit.NewRepo(t)
	wt := testkit.AddWorktree(t, repo, ".claude/worktrees/feat-a", "worktree-feat-a")

	for name, r := range map[string]result{
		"cwd inside worktree": run(t, wt, "scan", "--json"),
		"path argument":       run(t, testkit.TempDir(t), "scan", "--json", repo),
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, 0, r.code, r.stderr)
			var env struct {
				cli.Envelope
				Data cli.ScanResult `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
			assert.Equal(t, cli.SchemaVersion, env.SchemaVersion)
			assert.Equal(t, "ok", env.Status)
			assert.Equal(t, []string{}, env.Warnings)
			require.Len(t, env.Data.Repos, 1)
			assert.Equal(t, repo, env.Data.Repos[0].Root)
			require.Len(t, env.Data.Repos[0].Worktrees, 2)
			assert.Equal(t, wt, env.Data.Repos[0].Worktrees[1].Path)
		})
	}
}

func TestScanNotARepo(t *testing.T) {
	dir := testkit.TempDir(t)

	r := run(t, dir, "scan")
	assert.Equal(t, cli.ExitFailure, r.code)
	assert.Contains(t, r.stderr, "not a git repository")

	r = run(t, dir, "scan", "--json")
	assert.Equal(t, cli.ExitFailure, r.code)
	assert.Empty(t, r.stderr, "json errors are reported once, on stdout")
	var env cli.Envelope
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, "error", env.Status)
	require.NotNil(t, env.Error)
	assert.Equal(t, "not_a_repo", env.Error.Code)
}

func TestUsageErrors(t *testing.T) {
	dir := testkit.TempDir(t)
	for _, args := range [][]string{{"bogus"}, {"scan", "--nope"}, {"scan", "a", "b"}} {
		r := run(t, dir, args...)
		assert.Equal(t, cli.ExitUsage, r.code, "%v: %s", args, r.stderr)
	}
}

func TestVersion(t *testing.T) {
	r := run(t, testkit.TempDir(t), "version")
	require.Equal(t, 0, r.code)
	assert.Contains(t, r.stdout, "zap ")
}

func TestScanStatuses(t *testing.T) {
	repo := testkit.NewRepo(t)
	merged := testkit.AddWorktree(t, repo, ".claude/worktrees/done", "worktree-done")
	testkit.Commit(t, merged, "done.txt", "d\n", "done")
	testkit.MergeFF(t, repo, "worktree-done")
	open := testkit.AddWorktree(t, repo, ".worktrees/wip", "wip")
	testkit.Commit(t, open, "wip.txt", "w\n", "wip")
	testkit.Dirty(t, open)

	r := run(t, repo, "scan")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "target main")
	assert.Regexp(t, `\.claude/worktrees/done\s+worktree-done\s+merged`, r.stdout)
	assert.Regexp(t, `\.worktrees/wip\s+wip\s+active\s+\S+\s+\S+\s+dirty\(1\)`, r.stdout)
	assert.Contains(t, r.stdout, "1 merged · 1 active")
}

func TestExplain(t *testing.T) {
	repo := testkit.NewRepo(t)
	wt := testkit.AddWorktree(t, repo, ".claude/worktrees/done", "worktree-done")
	testkit.Commit(t, wt, "done.txt", "d\n", "done")
	testkit.MergeCommit(t, repo, "worktree-done")

	r := run(t, repo, "explain", wt)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "status   MERGED")
	assert.Regexp(t, `✓ ancestor\s+tip \w+ is contained in main`, r.stdout)
	assert.Regexp(t, `✓ empty-diff`, r.stdout, "explain runs every check")
	assert.Contains(t, r.stdout, "dirty          no")

	r = run(t, repo, "explain", "--json", ".claude/worktrees/done")
	require.Equal(t, 0, r.code, r.stderr)
	var env struct {
		cli.Envelope
		Data cli.ExplainResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, "merged", string(env.Data.Worktree.Verdict.Status))
	assert.Len(t, env.Data.Worktree.Verdict.Checks, 3)
}

func TestExplainMissingAndErrors(t *testing.T) {
	repo := testkit.NewRepo(t)
	gone := testkit.AddWorktree(t, repo, ".worktrees/gone", "gone")
	require.NoError(t, os.RemoveAll(gone))

	r := run(t, repo, "explain", gone)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "status   MISSING")

	r = run(t, repo, "explain", "--json", repo+"/README.md")
	assert.Equal(t, cli.ExitFailure, r.code)
	var env cli.Envelope
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, "not_a_worktree", env.Error.Code)

	r = run(t, repo, "scan", "--json", "--target", "nope")
	assert.Equal(t, cli.ExitFailure, r.code)
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, "no_target", env.Error.Code)

	assert.Equal(t, cli.ExitUsage, run(t, repo, "explain").code)
}
