// Package restore undoes a zap clean run from the operations log: it recreates deleted
// branches from their backup refs and re-adds removed worktrees at their old paths.
package restore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/oplog"
)

// ErrNothingToUndo is returned when no run has anything left to restore.
var ErrNothingToUndo = errors.New("nothing to undo")

// ErrUnknownRun is returned when the requested run is not in the log.
var ErrUnknownRun = errors.New("no such run in the history")

// Outcome is what undo did, or would do, for one zapped worktree.
type Outcome struct {
	Path   string   `json:"path"`
	Repo   string   `json:"repo"`
	Branch string   `json:"branch,omitempty"`
	Result string   `json:"result"` // oplog.Restored or oplog.RestoreFailed
	Steps  []string `json:"steps"`
	Detail string   `json:"detail,omitempty"`
}

// Options control Undo.
type Options struct {
	Run    string // empty means the most recent run with something left to restore
	DryRun bool
	Now    func() time.Time
}

// Pending returns, for run (or the latest run when empty), the zapped entries not yet
// restored, oldest first.
func Pending(entries []oplog.Entry, run string) (string, []oplog.Entry, error) {
	restored := map[string]bool{} // run|path
	for _, e := range entries {
		if e.Result == oplog.Restored {
			restored[e.Run+"|"+e.Path] = true
		}
	}
	pending := map[string][]oplog.Entry{}
	var order []string // newest run first, as entries are newest first
	for _, e := range entries {
		if e.Result != oplog.Zapped || restored[e.Run+"|"+e.Path] {
			continue
		}
		if _, seen := pending[e.Run]; !seen {
			order = append(order, e.Run)
		}
		pending[e.Run] = append([]oplog.Entry{e}, pending[e.Run]...)
	}
	if run == "" {
		if len(order) == 0 {
			return "", nil, ErrNothingToUndo
		}
		run = order[0]
	}
	if _, ok := pending[run]; !ok {
		for _, e := range entries {
			if e.Run == run {
				return run, nil, fmt.Errorf("restore: run %s: %w", run, ErrNothingToUndo)
			}
		}
		return run, nil, fmt.Errorf("restore: %s: %w", run, ErrUnknownRun)
	}
	return run, pending[run], nil
}

// Undo restores the pending entries of a run and records each result in the log.
func Undo(ctx context.Context, git gitx.Runner, log oplog.Log, opt Options) (string, []Outcome, error) {
	entries, _, err := log.Read()
	if err != nil {
		return "", nil, err
	}
	run, pending, err := Pending(entries, opt.Run)
	if err != nil {
		return run, nil, err
	}
	var outs []Outcome
	for _, e := range pending {
		if err := ctx.Err(); err != nil {
			return run, outs, err
		}
		o := restoreOne(ctx, git, e, opt.DryRun)
		outs = append(outs, o)
		if opt.DryRun {
			continue
		}
		rec := e
		rec.Time, rec.Result, rec.Detail, rec.BranchDeleted = opt.Now(), o.Result, strings.Join(o.Steps, "; "), false
		if o.Detail != "" {
			rec.Detail = o.Detail
		}
		if err := log.Append(rec); err != nil {
			return run, outs, err
		}
	}
	return run, outs, nil
}

func restoreOne(ctx context.Context, git gitx.Runner, e oplog.Entry, dryRun bool) Outcome {
	o := Outcome{Path: e.Path, Repo: e.Repo, Branch: e.Branch, Result: oplog.Restored}
	fail := func(err error) Outcome {
		o.Result, o.Detail = oplog.RestoreFailed, err.Error()
		return o
	}
	if _, err := os.Stat(e.Repo); err != nil {
		return fail(fmt.Errorf("repository is gone: %w", err))
	}
	source := e.BackupRef
	if source == "" {
		source = e.Head
	}

	if e.Branch != "" {
		switch {
		case gitx.BranchExists(ctx, git, e.Repo, e.Branch):
			o.Steps = append(o.Steps, "branch "+e.Branch+" already exists, left as is")
		case dryRun:
			o.Steps = append(o.Steps, "recreate branch "+e.Branch+" from "+source)
		default:
			if err := gitx.CreateBranch(ctx, git, e.Repo, e.Branch, source); err != nil {
				return fail(err)
			}
			o.Steps = append(o.Steps, "recreated branch "+e.Branch)
		}
	}

	if e.Status == string(model.StatusMissing) {
		o.Steps = append(o.Steps, "folder was already gone when zapped, not re-added")
		return o
	}
	if _, err := os.Stat(e.Path); err == nil {
		o.Steps = append(o.Steps, "path is in use, worktree not re-added")
		return o
	}
	if dryRun {
		o.Steps = append(o.Steps, "re-add worktree")
		return o
	}
	if err := gitx.AddWorktree(ctx, git, e.Repo, e.Path, e.Branch, source); err != nil {
		return fail(err)
	}
	o.Steps = append(o.Steps, "re-added worktree")
	return o
}
