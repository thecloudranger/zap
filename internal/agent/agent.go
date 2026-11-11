// Package agent guesses which tool created a worktree from where it lives on disk.
// It is a data table: supporting a new tool is one line.
package agent

import (
	"path/filepath"
	"strings"
)

// folders maps a path segment to the tool that keeps its worktrees there.
var folders = []struct {
	segment string
	name    string
}{
	{"/.claude/worktrees/", "claude"},
	{"/.codex/worktrees/", "codex"},
	{"/.cursor/worktrees/", "cursor"},
	{"/conductor/workspaces/", "conductor"},
	{"/.t3/worktrees/", "t3"},
}

// Detect returns the tool name for a worktree path, or "" when unknown.
func Detect(path string) string {
	p := filepath.ToSlash(path) + "/"
	for _, f := range folders {
		if strings.Contains(p, f.segment) {
			return f.name
		}
	}
	return ""
}

// Folders returns the per-user agent folders (relative to home) that hold worktrees
// outside any repository, for use as default scan roots.
func Folders() []string {
	return []string{".codex/worktrees", ".cursor/worktrees", "conductor/workspaces", ".t3/worktrees"}
}
