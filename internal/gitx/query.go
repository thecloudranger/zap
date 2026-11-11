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

// ReflogSHAs returns the commits a ref's reflog has pointed at, newest first. An empty
// result means the reflog is missing (reflogs disabled or expired).
func ReflogSHAs(ctx context.Context, r Runner, dir, ref string) ([]string, error) {
	out, err := r.Run(ctx, dir, "reflog", "show", "--format=%H", ref, "--")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// UpdateRef points ref at sha.
func UpdateRef(ctx context.Context, r Runner, dir, ref, sha string) error {
	_, err := r.Run(ctx, dir, "update-ref", ref, sha)
	return err
}

// RemoveWorktree runs `git worktree remove` without --force, so git itself refuses a
// worktree with uncommitted or untracked changes.
func RemoveWorktree(ctx context.Context, r Runner, dir, path string) error {
	_, err := r.Run(ctx, dir, "worktree", "remove", path)
	return err
}

// DeleteBranch force-deletes a local branch. Callers back it up first.
func DeleteBranch(ctx context.Context, r Runner, dir, branch string) error {
	_, err := r.Run(ctx, dir, "branch", "-D", "--", branch)
	return err
}

// RemoveAdmin unregisters one worktree whose folder is gone by deleting its admin
// directory; it is `git worktree prune` limited to a single worktree.
func RemoveAdmin(commonDir, wtPath string) error {
	if _, err := os.Stat(wtPath); err == nil {
		return fmt.Errorf("gitx: %s still exists; refusing to unregister it", wtPath)
	}
	admin, err := AdminDir(commonDir, wtPath)
	if err != nil {
		return err
	}
	if filepath.Dir(admin) != filepath.Join(commonDir, "worktrees") {
		return fmt.Errorf("gitx: unexpected admin dir %s", admin)
	}
	if err := os.RemoveAll(admin); err != nil {
		return fmt.Errorf("gitx: unregister worktree: %w", err)
	}
	return nil
}

// CherryAllApplied reports whether every commit of head that is not in target has an
// equivalent patch in target (`git cherry` marks them "-"): a rebase or cherry-pick merge.
// With no commits outside target it is trivially true.
func CherryAllApplied(ctx context.Context, r Runner, dir, target, head string) (bool, error) {
	out, err := r.Run(ctx, dir, "cherry", target, head)
	if err != nil {
		return false, err
	}
	lines := strings.Fields(string(out))
	for i := 0; i+1 < len(lines); i += 2 {
		if lines[i] != "-" {
			return false, nil
		}
	}
	return true, nil
}

// ErrUnsupported is returned when the installed git lacks a needed feature.
var ErrUnsupported = errors.New("not supported by this git version")

// MergeAddsNothing reports whether merging head into target would leave target's tree
// unchanged: head's changes are already there, as after a squash merge.
// Needs git 2.38+ (merge-tree --write-tree).
func MergeAddsNothing(ctx context.Context, r Runner, dir, target, head string) (bool, error) {
	out, err := r.Run(ctx, dir, "merge-tree", "--write-tree", "--no-messages", target, head)
	switch code := ExitCode(err); {
	case err == nil:
	case code == 1:
		return false, nil // conflicts: head has changes target does not
	case code == 129:
		return false, fmt.Errorf("gitx: merge-tree --write-tree: %w", ErrUnsupported)
	default:
		return false, err
	}
	merged, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	tree, err := r.Run(ctx, dir, "rev-parse", "--verify", target+"^{tree}")
	if err != nil {
		return false, err
	}
	return merged != "" && merged == strings.TrimSpace(string(tree)), nil
}

// UpstreamGone reports whether branch tracks an upstream that no longer exists,
// e.g. the remote branch was deleted after its pull request was merged.
func UpstreamGone(ctx context.Context, r Runner, dir, branch string) (upstream string, gone bool, err error) {
	out, err := r.Run(ctx, dir, "for-each-ref", "--format=%(upstream:short)%00%(upstream:track)", "refs/heads/"+branch)
	if err != nil {
		return "", false, err
	}
	up, track, _ := strings.Cut(strings.TrimSpace(string(out)), "\x00")
	return up, up != "" && track == "[gone]", nil
}

// Fetch updates all remotes and prunes deleted remote branches.
func Fetch(ctx context.Context, r Runner, dir string) error {
	_, err := r.Run(ctx, dir, "fetch", "--all", "--prune", "--quiet")
	return err
}

// BranchExists reports whether refs/heads/<branch> exists.
func BranchExists(ctx context.Context, r Runner, dir, branch string) bool {
	return refExists(ctx, r, dir, "refs/heads/"+branch)
}

// CreateBranch creates branch at sha (fails if it exists).
func CreateBranch(ctx context.Context, r Runner, dir, branch, sha string) error {
	_, err := r.Run(ctx, dir, "branch", "--", branch, sha)
	return err
}

// AddWorktree checks out branch at path, or sha detached when branch is empty.
func AddWorktree(ctx context.Context, r Runner, dir, path, branch, sha string) error {
	args := []string{"worktree", "add", "--quiet", path, branch}
	if branch == "" {
		args = []string{"worktree", "add", "--quiet", "--detach", path, sha}
	}
	_, err := r.Run(ctx, dir, args...)
	return err
}

// IgnoredEntries lists the ignored paths in a worktree as git reports them: an ignored
// directory such as node_modules/ is one entry and is not descended into.
func IgnoredEntries(ctx context.Context, r Runner, wt string) ([]string, error) {
	out, err := r.Run(ctx, wt, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory", "--no-empty-directory")
	if err != nil {
		return nil, err
	}
	var entries []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) > 0 {
			entries = append(entries, string(f))
		}
	}
	return entries, nil
}

// HiddenChanges counts files marked assume-unchanged, or skip-worktree outside a sparse
// checkout: git status does not show changes to them.
func HiddenChanges(ctx context.Context, r Runner, wt string) (int, error) {
	out, err := r.Run(ctx, wt, "ls-files", "-v", "-z")
	if err != nil {
		return 0, err
	}
	sparse := false
	if v, err := r.Run(ctx, wt, "config", "--bool", "core.sparseCheckout"); err == nil {
		sparse = strings.TrimSpace(string(v)) == "true"
	}
	n := 0
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) < 2 {
			continue
		}
		tag := f[0]
		switch {
		case tag >= 'a' && tag <= 'z': // lowercase: assume-unchanged
			n++
		case tag == 'S' && !sparse:
			n++
		}
	}
	return n, nil
}

// Unlock releases `git worktree lock` on path.
func Unlock(ctx context.Context, r Runner, dir, path string) error {
	_, err := r.Run(ctx, dir, "worktree", "unlock", path)
	return err
}
