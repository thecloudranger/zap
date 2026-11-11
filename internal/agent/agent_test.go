package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	for path, want := range map[string]string{
		"/Users/u/dev/app/.claude/worktrees/feat-a": "claude",
		"/Users/u/.codex/worktrees/1a2b/app":        "codex",
		"/Users/u/.cursor/worktrees/app/x":          "cursor",
		"/Users/u/conductor/workspaces/app/denver":  "conductor",
		"/Users/u/.t3/worktrees/app":                "t3",
		"/Users/u/dev/app/.worktrees/fix":           "",
		"/Users/u/dev/claude/worktrees-notes":       "",
		"/Users/u/dev/app/.claude/worktrees":        "claude",
	} {
		assert.Equal(t, want, Detect(path), path)
	}
}
