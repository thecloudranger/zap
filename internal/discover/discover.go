// Package discover walks directory trees and finds git repositories without running git:
// it reads .git directories and .git files to group every checkout by repository.
package discover

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// skipDirs are never descended into. Only names that are unambiguous are listed: a
// project called "build" or "target" must still be found.
var skipDirs = map[string]bool{
	"node_modules": true, ".venv": true, "__pycache__": true, ".next": true, ".nuxt": true,
	".turbo": true, ".gradle": true, ".cache": true, "Pods": true, "DerivedData": true,
	".Trash": true,
}

// Options control Find.
type Options struct {
	MaxDepth int // directory levels below each root; 0 means 6
	// Progress, if set, is called from walker goroutines with the number of
	// directories visited so far.
	Progress func(visited int)
}

// Result is what Find discovered.
type Result struct {
	Repos    []string `json:"repos"` // one directory per repository, its main worktree when known
	Warnings []string `json:"warnings,omitempty"`
	Visited  int      `json:"visited"`
}

// Find walks roots concurrently and returns one entry directory per repository.
// Unreadable directories become warnings; only cancellation is an error.
func Find(ctx context.Context, roots []string, opt Options) (Result, error) {
	if opt.MaxDepth <= 0 {
		opt.MaxDepth = 6
	}
	var (
		mu       sync.Mutex
		byCommon = map[string]string{} // common git dir -> entry dir
		isMainOf = map[string]bool{}
		res      Result
		wg       sync.WaitGroup
	)
	// Several checkouts share one repository; prefer its main worktree as the entry.
	add := func(common, entry string, isMain bool) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := byCommon[common]; !ok || (isMain && !isMainOf[common]) {
			byCommon[common], isMainOf[common] = entry, isMain
		}
	}
	warn := func(format string, a ...any) {
		mu.Lock()
		res.Warnings = append(res.Warnings, fmt.Sprintf(format, a...))
		mu.Unlock()
	}
	visit := func() {
		mu.Lock()
		res.Visited++
		n := res.Visited
		mu.Unlock()
		if opt.Progress != nil && n%200 == 0 {
			opt.Progress(n)
		}
	}

	home, _ := os.UserHomeDir()
	skipPath := filepath.Join(home, "Library") // macOS app data, never projects
	for _, root := range dedupeRoots(roots) {
		wg.Go(func() { walk(ctx, root, opt.MaxDepth, skipPath, add, warn, visit) })
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	for _, entry := range byCommon {
		res.Repos = append(res.Repos, entry)
	}
	sort.Strings(res.Repos)
	sort.Strings(res.Warnings)
	return res, nil
}

func walk(ctx context.Context, root string, maxDepth int, skipPath string, add func(common, entry string, isMain bool), warn func(string, ...any), visit func()) {
	base := strings.Count(root, string(filepath.Separator))
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			if p == root {
				warn("%s: %v", root, err)
				return filepath.SkipAll
			}
			warn("%s: %v", p, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".git" {
			if common, entry, isMain, ok := resolve(p, d); ok {
				add(common, entry, isMain)
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		visit()
		if p != root && (skipDirs[d.Name()] || p == skipPath || strings.Count(p, string(filepath.Separator))-base > maxDepth) {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		warn("%s: %v", root, err)
	}
}

// resolve maps a .git entry to its repository's common git dir and the best directory
// to run git in. isMain is true when the entry is the repository's main worktree.
func resolve(gitPath string, d fs.DirEntry) (common, entry string, isMain, ok bool) {
	dir := filepath.Dir(gitPath)
	if d.IsDir() {
		return realpath(gitPath), dir, true, true
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", "", false, false
	}
	gitdir, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !found {
		return "", "", false, false
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	common = gitdir
	if c, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
		cd := strings.TrimSpace(string(c))
		if !filepath.IsAbs(cd) {
			cd = filepath.Join(gitdir, cd)
		}
		common = cd
	}
	common = realpath(common)
	if _, err := os.Stat(common); err != nil {
		return "", "", false, false // dangling: its repository is gone
	}
	// A linked worktree of a normal repository: run git from the main worktree.
	if filepath.Base(common) == ".git" {
		if main := filepath.Dir(common); exists(main) {
			return common, main, true, true
		}
	}
	return common, dir, false, true
}

func realpath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// dedupeRoots drops missing roots and roots nested inside other roots.
func dedupeRoots(roots []string) []string {
	var real []string
	for _, r := range roots {
		if fi, err := os.Stat(r); err == nil && fi.IsDir() {
			real = append(real, realpath(r))
		}
	}
	sort.Strings(real)
	var out []string
	for _, r := range real {
		if len(out) > 0 {
			last := out[len(out)-1]
			if r == last || strings.HasPrefix(r, last+string(filepath.Separator)) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
