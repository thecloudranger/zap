// Package model holds the plain data types shared across zap. It imports nothing internal.
package model

import "time"

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
	CommonDir string     `json:"commonDir,omitempty"`
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

// Target is the branch "merged" is measured against. Refs are the existing refs for it,
// e.g. refs/heads/main and refs/remotes/origin/main; merged into any of them counts.
type Target struct {
	Name string   `json:"name"`
	Refs []string `json:"refs"`
}

// Status is zap's verdict on a worktree.
type Status string

// Statuses, in the order they are decided.
const (
	StatusMain    Status = "main"    // the repository's main worktree; never a candidate
	StatusMissing Status = "missing" // registered but its folder is gone
	StatusEmpty   Status = "empty"   // never got a commit of its own
	StatusMerged  Status = "merged"  // its work is already in the target
	StatusActive  Status = "active"  // anything else
)

// Outcome is the result of one check.
type Outcome string

// Outcomes. Error never counts as a match: doubt means keep.
const (
	Match   Outcome = "match"
	NoMatch Outcome = "no-match"
	Skipped Outcome = "skipped"
	Errored Outcome = "error"
	Pass    Outcome = "pass" // safety gates pass or fail
	Fail    Outcome = "fail"
)

// CheckResult records what one check found, for `zap explain`.
type CheckResult struct {
	Check   string  `json:"check"`
	Outcome Outcome `json:"outcome"`
	Detail  string  `json:"detail"`
}

// Verdict is the decided status plus the evidence behind it.
type Verdict struct {
	Status Status        `json:"status"`
	Reason string        `json:"reason"`
	Checks []CheckResult `json:"checks,omitempty"`
}

// Facts are observations about a worktree that do not decide its status.
type Facts struct {
	Dirty        bool      `json:"dirty"`
	Changes      int       `json:"changes"` // modified + untracked paths (ignored files excluded)
	Created      time.Time `json:"created,omitzero"`
	LastCommit   time.Time `json:"lastCommit,omitzero"`
	LastActivity time.Time `json:"lastActivity,omitzero"`
}

// Assessment is a worktree with its facts and verdict.
type Assessment struct {
	Worktree
	Facts   Facts    `json:"facts"`
	Verdict Verdict  `json:"verdict"`
	Errors  []string `json:"errors,omitempty"`
}

// RepoReport is everything zap knows about one repository.
type RepoReport struct {
	Root      string       `json:"root"`
	CommonDir string       `json:"commonDir,omitempty"`
	Target    Target       `json:"target"`
	Worktrees []Assessment `json:"worktrees"`
}
