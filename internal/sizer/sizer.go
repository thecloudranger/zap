// Package sizer measures how much disk removing a worktree would free: allocated blocks of
// files with a single link, split into dependencies and everything else. Hard-linked
// files (a pnpm store, for example) are reported separately as shared.
package sizer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"

	"github.com/thecloudranger/zap/internal/model"
)

// depDirs are folders of rebuildable dependencies or build output. The split only labels
// sizes; it never decides what is safe to delete.
var depDirs = map[string]bool{
	"node_modules": true, ".venv": true, "venv": true, "target": true, "dist": true,
	"build": true, ".next": true, ".nuxt": true, ".turbo": true, ".gradle": true,
	"__pycache__": true, ".cache": true, "Pods": true, "DerivedData": true, "vendor": true,
}

// Counter sizes directory trees. It holds no state, so results do not depend on the
// order worktrees are measured in; it is safe for concurrent use.
type Counter struct{}

// NewCounter returns a Counter.
func NewCounter() *Counter { return &Counter{} }

// Size walks root without following symlinks. Unreadable entries are skipped.
func (c *Counter) Size(ctx context.Context, root string) (model.Usage, error) {
	var u model.Usage
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if p == root {
				return err
			}
			return nil // unreadable: skip
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		n, shared := c.bytes(info)
		switch {
		case shared:
			u.Shared += n
		default:
			u.Total += n
			if inDeps(root, p) {
				u.Deps += n
			}
		}
		return nil
	})
	if err != nil {
		return model.Usage{}, fmt.Errorf("sizer: %s: %w", root, err)
	}
	return u, nil
}

// bytes is the allocated size of one entry and whether it is a hard-linked file.
func (c *Counter) bytes(info fs.FileInfo) (int64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size(), false
	}
	n := int64(st.Blocks) * 512                     // st_blocks is in 512-byte units on darwin and linux
	return n, !info.IsDir() && uint64(st.Nlink) > 1 // Nlink's width differs by platform
}

// inDeps reports whether p is inside a dependency folder below root.
func inDeps(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	for dir := filepath.Dir(rel); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if depDirs[filepath.Base(dir)] {
			return true
		}
	}
	return depDirs[filepath.Base(rel)]
}

// ErrNoStatfs is returned when free space cannot be read.
var ErrNoStatfs = errors.New("free space unavailable")

// Human formats a byte count in decimal units, as Finder does: 999 B, 1.2 MB, 45 GB.
func Human(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f, i := float64(n), 0
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	for f /= unit; f >= unit && i < len(units)-1; f /= unit {
		i++
	}
	if f < 10 {
		return fmt.Sprintf("%.1f %s", f, units[i])
	}
	return fmt.Sprintf("%.0f %s", f, units[i])
}
