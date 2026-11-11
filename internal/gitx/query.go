package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thecloudranger/zap/internal/model"
)

// ErrNoTarget is returned when no target branch can be resolved.
var ErrNoTarget = errors.New("no target branch found")

// CommonDir returns the absolute path of the repository's shared git directory.
func CommonDir(ctx context.Context, r Runner, dir string) (string, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveTarget picks the branch "merged" is measured against. With name empty it uses
// origin/HEAD, then main, then master. Both the local and origin refs count when they exist.
func ResolveTarget(ctx context.Context, r Runner, dir, name string) (model.Target, error) {
	if name == "" {
		name = defaultBranch(ctx, r, dir)
	}
	if name == "" {
		return model.Target{}, fmt.Errorf("gitx: %w (looked for origin/HEAD, main, master); pass --target", ErrNoTarget)
	}
	t := model.Target{Name: name}
	for _, ref := range []string{"refs/heads/" + name, "refs/remotes/origin/" + name} {
		if refExists(ctx, r, dir, ref) {
			t.Refs = append(t.Refs, ref)
		}
	}
	if len(t.Refs) == 0 {
		return model.Target{}, fmt.Errorf("gitx: target %q: %w", name, ErrNoTarget)
	}
	return t, nil
}

func defaultBranch(ctx context.Context, r Runner, dir string) string {
	if out, err := r.Run(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "origin/"); ok && name != "" {
			return name
		}
	}
	for _, name := range []string{"main", "master"} {
		if refExists(ctx, r, dir, "refs/heads/"+name) || refExists(ctx, r, dir, "refs/remotes/origin/"+name) {
			return name
		}
	}
	return ""
}

func refExists(ctx context.Context, r Runner, dir, ref string) bool {
	_, err := r.Run(ctx, dir, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

// IsAncestor reports whether commit a is reachable from b.
func IsAncestor(ctx context.Context, r Runner, dir, a, b string) (bool, error) {
	_, err := r.Run(ctx, dir, "merge-base", "--is-ancestor", a, b)
	return yesNo(err)
}

// DiffEmpty reports whether `target...head` has no changes, i.e. head adds nothing
// relative to its merge base with target.
func DiffEmpty(ctx context.Context, r Runner, dir, target, head string) (bool, error) {
	_, err := r.Run(ctx, dir, "diff", "--quiet", target+"..."+head, "--")
	return yesNo(err)
}

// yesNo maps git's exit 0 / exit 1 answer to a bool; anything else is an error.
func yesNo(err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case ExitCode(err) == 1:
		return false, nil
	default:
		return false, err
	}
}

// Changes counts modified, staged and untracked paths in a worktree (ignored files excluded).
func Changes(ctx context.Context, r Runner, wt string) (int, error) {
	out, err := r.Run(ctx, wt, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return 0, err
	}
	n := 0
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 3 {
			continue
		}
		n++
		if f[0] == 'R' || f[0] == 'C' { // renames and copies carry the source path as the next field
			i++
		}
	}
	return n, nil
}

// LastCommit returns the committer time of HEAD in a worktree.
func LastCommit(ctx context.Context, r Runner, wt string) (time.Time, error) {
	out, err := r.Run(ctx, wt, "log", "-1", "--format=%ct", "HEAD")
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("gitx: parse commit time: %w", err)
	}
	return time.Unix(sec, 0), nil
}

// NewestFileTime returns the latest mtime among tracked and untracked, non-ignored files,
// so a running build in node_modules does not count as activity.
func NewestFileTime(ctx context.Context, r Runner, wt string) (time.Time, error) {
	out, err := r.Run(ctx, wt, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return time.Time{}, err
	}
	var newest time.Time
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) == 0 {
			continue
		}
		if ctx.Err() != nil {
			return time.Time{}, ctx.Err()
		}
		fi, err := os.Lstat(filepath.Join(wt, string(f)))
		if err != nil {
			continue // deleted in the working tree
		}
		if mt := fi.ModTime(); mt.After(newest) {
			newest = mt
		}
	}
	return newest, nil
}

// Created returns when a linked worktree was added: the mtime of the `commondir` file in
// its admin directory, which git writes once at creation and never rewrites.
func Created(commonDir, wtPath string) (time.Time, error) {
	admin, err := AdminDir(commonDir, wtPath)
	if err != nil {
		return time.Time{}, err
	}
	fi, err := os.Stat(filepath.Join(admin, "commondir"))
	if err != nil {
		return time.Time{}, fmt.Errorf("gitx: worktree created time: %w", err)
	}
	return fi.ModTime(), nil
}

// AdminDir finds <commonDir>/worktrees/<id> for a worktree path by matching each admin
// dir's gitdir file, so it works even when the worktree folder is gone.
func AdminDir(commonDir, wtPath string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(commonDir, "worktrees"))
	if err != nil {
		return "", fmt.Errorf("gitx: read worktrees admin dir: %w", err)
	}
	want := filepath.Clean(filepath.Join(wtPath, ".git"))
	for _, e := range entries {
		dir := filepath.Join(commonDir, "worktrees", e.Name())
		b, err := os.ReadFile(filepath.Join(dir, "gitdir"))
		if err != nil {
			continue
		}
		if filepath.Clean(strings.TrimSpace(string(b))) == want {
			return dir, nil
		}
	}
	return "", fmt.Errorf("gitx: no admin dir for worktree %s", wtPath)
}
