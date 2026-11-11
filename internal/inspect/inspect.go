// Package inspect turns a repository into a RepoReport: worktrees, facts and verdicts.
package inspect

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/verdict"
)

// Options control an inspection.
type Options struct {
	Target   string         // target branch override; empty resolves the default
	AllRules bool           // run every check (explain) instead of stopping at the first match
	Fetch    bool           // run `git fetch --all --prune` first
	Rules    []verdict.Rule // nil means verdict.Default()
}

// Repo inspects the repository containing dir.
func Repo(ctx context.Context, git gitx.Runner, dir string, opt Options) (model.RepoReport, error) {
	repo, err := gitx.ListRepo(ctx, git, dir)
	if err != nil {
		return model.RepoReport{}, err
	}
	if repo.CommonDir, err = gitx.CommonDir(ctx, git, repo.Root); err != nil {
		return model.RepoReport{}, err
	}
	var warnings []string
	if opt.Fetch {
		if err := gitx.Fetch(ctx, git, repo.Root); err != nil {
			if ctx.Err() != nil {
				return model.RepoReport{}, ctx.Err()
			}
			warnings = append(warnings, "fetch failed, using the last fetched state: "+err.Error())
		}
	}
	target, err := gitx.ResolveTarget(ctx, git, repo.Root, opt.Target)
	if err != nil {
		return model.RepoReport{}, err
	}
	rules := opt.Rules
	if rules == nil {
		rules = verdict.Default()
	}

	out := make([]model.Assessment, len(repo.Worktrees))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.GOMAXPROCS(0))
	for i, wt := range repo.Worktrees {
		g.Go(func() error {
			s := &verdict.Subject{Git: git, Repo: repo, WT: wt, Target: target}
			a := model.Assessment{Worktree: wt, Verdict: verdict.Evaluate(gctx, s, rules, opt.AllRules)}
			a.Facts, a.Errors = facts(gctx, git, repo.CommonDir, wt)
			out[i] = a
			return gctx.Err() // only cancellation aborts; per-worktree problems are recorded
		})
	}
	if err := g.Wait(); err != nil {
		return model.RepoReport{}, fmt.Errorf("inspect: %w", err)
	}
	return model.RepoReport{Root: repo.Root, CommonDir: repo.CommonDir, Target: target, Worktrees: out, Warnings: warnings}, nil
}

// facts gathers observations; failures are recorded rather than aborting the scan.
func facts(ctx context.Context, git gitx.Runner, commonDir string, wt model.Worktree) (model.Facts, []string) {
	var f model.Facts
	var errs []string
	note := func(what string, err error) {
		errs = append(errs, fmt.Sprintf("%s: %v", what, err))
	}
	if !wt.Main && !wt.Bare {
		if c, err := gitx.Created(commonDir, wt.Path); err != nil {
			note("created", err)
		} else {
			f.Created = c
		}
	}
	if wt.Prunable || wt.Bare {
		return f, errs
	}
	if n, err := gitx.Changes(ctx, git, wt.Path); err != nil {
		note("status", err)
	} else {
		f.Changes, f.Dirty = n, n > 0
	}
	if t, err := gitx.LastCommit(ctx, git, wt.Path); err != nil {
		note("last commit", err)
	} else {
		f.LastCommit = t
	}
	newest, err := gitx.NewestFileTime(ctx, git, wt.Path)
	if err != nil {
		note("file times", err)
	}
	f.LastActivity = latest(f.Created, f.LastCommit, newest)
	return f, errs
}

func latest(ts ...time.Time) time.Time {
	var m time.Time
	for _, t := range ts {
		if t.After(m) {
			m = t
		}
	}
	return m
}
