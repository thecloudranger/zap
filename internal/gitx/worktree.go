package gitx

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/thecloudranger/zap/internal/model"
)

// ListRepo returns the repository containing dir with all its worktrees.
func ListRepo(ctx context.Context, r Runner, dir string) (model.Repo, error) {
	out, err := r.Run(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return model.Repo{}, err
	}
	wts, err := ParseWorktreeList(out)
	if err != nil {
		return model.Repo{}, err
	}
	if len(wts) == 0 {
		return model.Repo{}, fmt.Errorf("gitx: worktree list in %s: empty output", dir)
	}
	return model.Repo{Root: wts[0].Path, Worktrees: wts}, nil
}

// ParseWorktreeList parses `git worktree list --porcelain -z`. Attributes are NUL-terminated and
// records end with an empty attribute. The first record is the main worktree.
func ParseWorktreeList(data []byte) ([]model.Worktree, error) {
	var (
		out []model.Worktree
		cur *model.Worktree
	)
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, field := range bytes.Split(data, []byte{0}) {
		line := string(field)
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		if key == "worktree" {
			flush()
			cur = &model.Worktree{Path: val, Main: len(out) == 0}
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("gitx: parse worktree list: %q before any worktree", line)
		}
		switch key {
		case "HEAD":
			cur.Head = val
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "detached":
			cur.Detached = true
		case "bare":
			cur.Bare = true
		case "locked":
			cur.Locked, cur.LockReason = true, val
		case "prunable":
			cur.Prunable, cur.PruneReason = true, val
		}
		// Unknown attributes are ignored so newer git versions keep working.
	}
	flush()
	return out, nil
}
