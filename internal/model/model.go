// Package model holds the plain data types shared across zap. It imports nothing internal.
package model

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path        string `json:"path"`
	Head        string `json:"head,omitempty"`
	Branch      string `json:"branch,omitempty"` // short name; empty when detached or bare
	Main        bool   `json:"main"`             // the repository's main working tree
	Detached    bool   `json:"detached,omitempty"`
	Bare        bool   `json:"bare,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	LockReason  string `json:"lockReason,omitempty"`
	Prunable    bool   `json:"prunable,omitempty"`
	PruneReason string `json:"pruneReason,omitempty"`
}

// Repo is a git repository and its worktrees. Root is the main worktree path.
type Repo struct {
	Root      string     `json:"root"`
	Worktrees []Worktree `json:"worktrees"`
}

// Linked returns the worktrees other than the main one.
func (r Repo) Linked() []Worktree {
	out := make([]Worktree, 0, len(r.Worktrees))
	for _, w := range r.Worktrees {
		if !w.Main {
			out = append(out, w)
		}
	}
	return out
}
