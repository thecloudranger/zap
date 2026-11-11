// Package testkit builds real git fixture repositories for tests. Every helper runs git
// with an isolated, fixed environment so results never depend on the developer's config.
package testkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Env is the hermetic git environment used by fixtures and by e2e runs of the binary.
func Env(home string) []string {
	keep := []string{}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k == "PATH" || k == "TMPDIR" {
			keep = append(keep, kv)
		}
	}
	return append(keep,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=zap-test", "GIT_AUTHOR_EMAIL=zap@test.invalid",
		"GIT_COMMITTER_NAME=zap-test", "GIT_COMMITTER_EMAIL=zap@test.invalid",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
}

// TempDir returns t.TempDir with symlinks resolved, because git reports real paths
// (on macOS /var is a symlink to /private/var).
func TempDir(t testing.TB) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Git runs git in dir and returns trimmed stdout, failing the test on error.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = Env(dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// NewRepo creates a repository on branch main with one commit and returns its path.
func NewRepo(t testing.TB) string {
	t.Helper()
	dir := filepath.Join(TempDir(t), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	Git(t, dir, "init", "-q", "-b", "main")
	Commit(t, dir, "README.md", "fixture\n", "initial")
	return dir
}

// Commit writes file with content in dir and commits it.
func Commit(t testing.TB, dir, file, content, msg string) {
	t.Helper()
	p := filepath.Join(dir, file)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	Git(t, dir, "add", "--", file)
	Git(t, dir, "commit", "-q", "-m", msg)
}

// AddWorktree creates branch at repo/<rel> as a linked worktree and returns its path.
// Use rel like ".claude/worktrees/x" to mimic an agent layout.
func AddWorktree(t testing.TB, repo, rel, branch string) string {
	t.Helper()
	p := filepath.Join(repo, rel)
	Git(t, repo, "worktree", "add", "-q", "-b", branch, p)
	return p
}
