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
	assert.Regexp(t, `WORKTREE\s+BRANCH\s+AGENT\s+STATUS\s+SIZE\s+DEPS\s+AGE\s+ACTIVE\s+NOTES`, r.stdout)
	assert.Regexp(t, `\d+(\.\d)? KB`, r.stdout, "worktrees are measured")
	assert.Regexp(t, `\.claude/worktrees/feat-a\s+worktree-feat-a\s+claude\s`, r.stdout)
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

	r := run(t, testkit.TempDir(t), "scan", dir)
	assert.Equal(t, cli.ExitFailure, r.code, "an explicit PATH must be a repository")
	assert.Contains(t, r.stderr, "not a git repository")

	r = run(t, testkit.TempDir(t), "scan", "--json", dir)
	assert.Equal(t, cli.ExitFailure, r.code)
	assert.Empty(t, r.stderr, "json errors are reported once, on stdout")
	var env cli.Envelope
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Equal(t, "error", env.Status)
	require.NotNil(t, env.Error)
	assert.Equal(t, "not_a_repo", env.Error.Code)

	r = run(t, dir, "scan")
	assert.Equal(t, 0, r.code, "outside a repository, scan searches the scan roots")
	assert.Contains(t, r.stdout, "Scanned 0 repositories")
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
	assert.Regexp(t, `\.claude/worktrees/done\s+worktree-done\s+claude\s+merged`, r.stdout)
	assert.Regexp(t, `\.worktrees/wip\s+wip\s+-\s+active\s+(\S+ \S+\s+){2}\S+\s+\S+\s+dirty\(1\)`, r.stdout)
	assert.Regexp(t, `1 merged · 1 active  ·  \S+ \S+, \S+ \S+ merged or empty`, r.stdout)

	r = run(t, repo, "scan", "--no-size")
	assert.Regexp(t, `merged\s+-\s+-\s`, r.stdout, "--no-size leaves sizes blank")
	assert.Contains(t, r.stdout, "1 merged · 1 active")

	testkit.AddWorktree(t, repo, ".claude/worktrees/fresh", "worktree-fresh")
	r = run(t, repo, "scan")
	assert.Regexp(t, `\.claude/worktrees/fresh\s+worktree-fresh\s+claude\s+empty`, r.stdout)
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
	assert.Len(t, env.Data.Worktree.Verdict.Checks, 8)
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
	assert.Regexp(t, `✓\s+\.claude/worktrees/done\s+worktree-done\s+merged\s+\S+ \S+\s+remove worktree, delete branch`, r.stdout)
	assert.Contains(t, r.stdout, "Dry run: nothing was changed. Zapping would free about")
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
	assert.Positive(t, env.Data.Results[0].Freed)
	require.NotNil(t, env.Data.Plans[0].Items[0].Size)
	assert.NoDirExists(t, wt)
	backup := env.Data.Results[0].BackupRef
	assert.Equal(t, env.Data.Plans[0].Items[0].Head, testkit.Git(t, repo, "rev-parse", backup), "backup ref holds the old tip")

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

	r = zap("undo")
	require.Equal(t, 0, r.code, r.stdout+r.stderr)
	assert.Contains(t, r.stdout, "recreated branch worktree-done")
	assert.Contains(t, r.stdout, "re-added worktree")
	assert.DirExists(t, wt)
	assert.Equal(t, "worktree-done", testkit.Git(t, wt, "branch", "--show-current"))

	r = zap("undo")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stdout, "Nothing to undo.")

	r = zap("undo", "--json", "20990101T000000Z")
	assert.Equal(t, cli.ExitFailure, r.code)
	var uerr cli.Envelope
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &uerr))
	assert.Equal(t, "unknown_run", uerr.Error.Code)

	r = zap("history")
	assert.Contains(t, r.stdout, "restored")
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

func TestScanAll(t *testing.T) {
	home := testkit.TempDir(t)
	mk := func(rel string) string {
		p := filepath.Join(home, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		testkit.Git(t, filepath.Dir(p), "init", "-q", "-b", "main", filepath.Base(p))
		testkit.Commit(t, p, "README.md", "x\n", "init")
		return p
	}
	app := mk("dev/app")
	testkit.AddWorktree(t, app, ".claude/worktrees/feat", "worktree-feat")
	mk("dev/quiet") // no worktrees: counted but not listed
	lib := mk("elsewhere/lib")
	testkit.Git(t, lib, "worktree", "add", "-q", "-b", "agent-1", filepath.Join(home, ".codex", "worktrees", "1", "lib"))

	r := runHome(t, home, home, "scan", "--all")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "~/dev/app  ·  1 linked worktree")
	assert.Contains(t, r.stdout, "~/elsewhere/lib  ·  1 linked worktree", "found through the codex agent folder")
	assert.Regexp(t, `~/\.codex/worktrees/1/lib\s+agent-1\s+codex\s`, r.stdout)
	assert.NotContains(t, r.stdout, "quiet")
	assert.Contains(t, r.stdout, "Scanned 3 repositories")

	r = runHome(t, home, home, "scan", "--all", "--json")
	require.Equal(t, 0, r.code)
	var env struct {
		Data cli.ScanResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.True(t, env.Data.All)
	assert.Len(t, env.Data.Repos, 3)

	// Custom roots replace the defaults.
	require.Equal(t, 0, runHome(t, home, home, "roots", "add", filepath.Join(home, "elsewhere")).code)
	require.Equal(t, 0, runHome(t, home, home, "roots", "remove", "~/dev").code)
	r = runHome(t, home, home, "roots")
	assert.Contains(t, r.stdout, "~/elsewhere")
	assert.NotContains(t, r.stdout, "~/dev")
	r = runHome(t, home, home, "scan", "--all")
	assert.NotContains(t, r.stdout, "~/dev/app")
	assert.Contains(t, r.stdout, "~/elsewhere/lib")
	assert.Equal(t, cli.ExitUsage, runHome(t, home, home, "roots", "remove", "~/nope").code)
	assert.Equal(t, 0, runHome(t, home, home, "roots", "reset").code)
	assert.Contains(t, runHome(t, home, home, "roots").stdout, "These are the defaults")

	assert.Equal(t, cli.ExitUsage, runHome(t, home, home, "scan", "--all", app).code)
}

func TestCleanAll(t *testing.T) {
	home := testkit.TempDir(t)
	acceptDisclaimer(t, home)
	merged := func(rel string) (repo, wt string) {
		repo = filepath.Join(home, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(repo), 0o755))
		testkit.Git(t, filepath.Dir(repo), "init", "-q", "-b", "main", filepath.Base(repo))
		testkit.Commit(t, repo, "README.md", "x\n", "init")
		wt = testkit.AddWorktree(t, repo, ".claude/worktrees/done", "worktree-done")
		testkit.Commit(t, wt, "d.txt", "d\n", "done")
		testkit.MergeFF(t, repo, "worktree-done")
		return repo, wt
	}
	a, aWT := merged("dev/a")
	b, bWT := merged("dev/b")
	_, cWT := merged("dev/c") // not opted in
	bad, badWT := merged("dev/bad")
	for _, r := range []string{a, b} {
		require.NoError(t, os.WriteFile(filepath.Join(r, ".zap"), []byte(`min_age = "0"`), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(bad, ".zap"), []byte(`min_agee = "0"`), 0o644))
	zap := func(args ...string) result { return runHome(t, home, home, args...) }

	r := zap("clean", "--all", "--dry-run")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "~/dev/a  ·  target main  ·  opted in")
	assert.Contains(t, r.stdout, "~/dev/b  ·  target main  ·  opted in")
	assert.Contains(t, r.stdout, "~/dev/c  ·  1 candidate, 0 passing the safety checks now")
	assert.Regexp(t, `warning: ~/dev/bad: skipped: .*min_agee`, r.stdout)
	assert.Contains(t, r.stdout, "2 to zap in 2 opted-in repositories.")

	assert.Equal(t, cli.ExitRefused, zap("clean", "--all").code, "non-interactive needs --yes")
	assert.Equal(t, cli.ExitUsage, zap("clean", "--all", a).code)

	r = zap("clean", "--all", "--yes", "--json")
	require.Equal(t, 0, r.code, r.stdout+r.stderr)
	var env struct {
		cli.Envelope
		Data cli.CleanResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &env))
	assert.Len(t, env.Data.Results, 2)
	assert.NotEmpty(t, env.Warnings, "the invalid .zap is reported")
	assert.NoDirExists(t, aWT)
	assert.NoDirExists(t, bWT)
	assert.DirExists(t, cWT, "not opted in")
	assert.DirExists(t, badWT, "invalid .zap means hands off")

	r = zap("undo")
	require.Equal(t, 0, r.code, r.stderr)
	assert.DirExists(t, aWT, "one undo restores the whole multi-repo run")
	assert.DirExists(t, bWT)
}
