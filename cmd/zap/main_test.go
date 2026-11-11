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
	return runHome(t, testkit.TempDir(t), dir, args...)
}

// runHome runs zap with a given HOME, so config and history persist across calls.
func runHome(t *testing.T, home, dir string, args ...string) result {
	t.Helper()
	cmd := exec.Command(zapBin, args...)
	cmd.Dir = dir
	cmd.Env = testkit.Env(home)
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

	testkit.AddWorktree(t, repo, ".claude/worktrees/fresh", "worktree-fresh")
	r = run(t, repo, "scan")
	assert.Regexp(t, `\.claude/worktrees/fresh\s+worktree-fresh\s+empty`, r.stdout)
	assert.Contains(t, r.stdout, "1 merged · 1 empty · 1 active")
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
	assert.Len(t, env.Data.Worktree.Verdict.Checks, 7)
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

func TestCleanNonInteractive(t *testing.T) {
	home := testkit.TempDir(t)
	repo := testkit.NewRepo(t)
	wt := testkit.AddWorktree(t, repo, ".claude/worktrees/done", "worktree-done")
	testkit.Commit(t, wt, "d.txt", "d\n", "done")
	testkit.MergeFF(t, repo, "worktree-done")
	zap := func(args ...string) result { return runHome(t, home, repo, args...) }

	r := zap("clean")
	assert.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "not opted in")
	assert.DirExists(t, wt)

	require.Equal(t, 0, zap("init").code)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".zap"), []byte(`min_age = "0"`), 0o644))

	r = zap("clean", "--dry-run")
	assert.Equal(t, 0, r.code, r.stderr)
	assert.Regexp(t, `✓\s+\.claude/worktrees/done\s+worktree-done\s+merged\s+remove worktree, delete branch`, r.stdout)
	assert.Contains(t, r.stdout, "Dry run: nothing was changed.")
	assert.DirExists(t, wt)

	assert.Equal(t, cli.ExitUsage, zap("clean", "--dry-run", "--yes").code)

	r = zap("clean", "--yes")
	assert.Equal(t, cli.ExitRefused, r.code, "disclaimer must be accepted on a terminal first")
	assert.Contains(t, r.stderr, "disclaimer")
	assert.DirExists(t, wt)

	acceptDisclaimer(t, home)
	r = zap("clean")
	assert.Equal(t, cli.ExitRefused, r.code)
	assert.Contains(t, r.stderr, "pass --yes")
	assert.DirExists(t, wt)

	r = zap("clean", "--yes", "--json")
	require.Equal(t, 0, r.code, r.stdout+r.stderr)
	var env struct {
		cli.Envelope
		Data cli.CleanResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	require.Len(t, env.Data.Results, 1)
	assert.Equal(t, "zapped", env.Data.Results[0].Outcome)
	assert.True(t, env.Data.Results[0].BranchDeleted)
	assert.NoDirExists(t, wt)
	backup := env.Data.Results[0].BackupRef
	assert.Equal(t, env.Data.Plan.Items[0].Head, testkit.Git(t, repo, "rev-parse", backup), "backup ref holds the old tip")

	r = zap("history", "--json")
	require.Equal(t, 0, r.code)
	var hist struct {
		Data cli.HistoryResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &hist))
	require.Len(t, hist.Data.Entries, 1)
	assert.Equal(t, wt, hist.Data.Entries[0].Path)

	r = zap("clean", "--yes")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stdout, "Nothing to zap")
}

func TestCleanJSONErrors(t *testing.T) {
	home := testkit.TempDir(t)
	repo := testkit.NewRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".zap"), []byte(`min_agee = "1d"`), 0o644))

	r := runHome(t, home, repo, "clean", "--json")
	assert.Equal(t, cli.ExitFailure, r.code)
	var env cli.Envelope
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, "invalid_zap", env.Error.Code)
	assert.Contains(t, env.Error.Message, "min_agee")
}

func acceptDisclaimer(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "zap")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("[disclaimer]\nversion = 1\naccepted_at = 2026-11-11T00:00:00Z\n"), 0o644))
}
