// Package leftovers finds the data coding agents keep outside worktrees: Claude Code's
// per-session temp folders and transcripts, and Codex's session logs. It only reports;
// nothing here deletes.
package leftovers

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Kind says what a group of leftovers is, and so how freely it could go.
type Kind string

const (
	Temp        Kind = "temp"        // scratch files of agent sessions
	Transcripts Kind = "transcripts" // conversation logs an agent needs to resume a session
)

// Item is one project's share of a group, or the group's only entry.
type Item struct {
	Label    string    `json:"label"` // ~/dev/api, or the raw folder name when it cannot be resolved
	Path     string    `json:"path"`
	Bytes    int64     `json:"bytes"`
	Sessions int       `json:"sessions"`
	LastUsed time.Time `json:"lastUsed"`
}

// Group is one agent's data of one kind.
type Group struct {
	Agent    string    `json:"agent"` // claude, codex
	Kind     Kind      `json:"kind"`
	Path     string    `json:"path"`
	Bytes    int64     `json:"bytes"`
	Sessions int       `json:"sessions"`
	LastUsed time.Time `json:"lastUsed"`
	Note     string    `json:"note"`
	Items    []Item    `json:"items,omitempty"` // per project, largest first
}

// Options says where to look. Empty fields take the real locations.
type Options struct {
	Home    string
	TmpRoot string // where Claude Code puts claude-<uid>; default /tmp
	UID     int    // default os.Getuid()
}

// Find returns the groups that exist, largest first. A location that is missing or
// unreadable is left out; a cancelled context stops the walk.
func Find(ctx context.Context, opt Options) ([]Group, error) {
	if opt.TmpRoot == "" {
		opt.TmpRoot = "/tmp"
	}
	if opt.UID == 0 {
		opt.UID = os.Getuid()
	}
	specs := []struct {
		agent, path string
		kind        Kind
		perProject  bool
		note        string
	}{
		{"claude", filepath.Join(opt.TmpRoot, "claude-"+strconv.Itoa(opt.UID)), Temp, true,
			"scratch files of Claude Code sessions"},
		{"claude", filepath.Join(opt.Home, ".claude", "projects"), Transcripts, true,
			"needed to resume Claude Code sessions"},
		{"codex", filepath.Join(opt.Home, ".codex", "sessions"), Transcripts, false,
			"needed to resume Codex sessions"},
	}
	var out []Group
	for _, sp := range specs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root, err := filepath.EvalSymlinks(sp.path) // /tmp is a symlink on macOS
		if err != nil {
			continue
		}
		g := Group{Agent: sp.agent, Kind: sp.kind, Path: root, Note: sp.note}
		if sp.perProject {
			entries, err := os.ReadDir(root)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				p := filepath.Join(root, e.Name())
				it, err := measure(ctx, p, sp.kind)
				if err != nil {
					return nil, err
				}
				it.Label = projectLabel(e.Name(), opt.Home)
				g.add(it)
			}
		} else {
			it, err := measure(ctx, root, sp.kind)
			if err != nil {
				return nil, err
			}
			g.Bytes, g.Sessions, g.LastUsed = it.Bytes, it.Sessions, it.LastUsed
		}
		if g.Bytes == 0 {
			continue
		}
		sort.Slice(g.Items, func(i, j int) bool { return g.Items[i].Bytes > g.Items[j].Bytes })
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out, nil
}

func (g *Group) add(it Item) {
	g.Items = append(g.Items, it)
	g.Bytes += it.Bytes
	g.Sessions += it.Sessions
	if it.LastUsed.After(g.LastUsed) {
		g.LastUsed = it.LastUsed
	}
}

// Total sums the groups.
func Total(gs []Group) int64 {
	var n int64
	for _, g := range gs {
		n += g.Bytes
	}
	return n
}

// measure walks dir for its allocated size and the newest file modification. Sessions counts,
// for temp data, the session folders directly inside dir; for transcripts, the
// .jsonl files anywhere below it.
func measure(ctx context.Context, dir string, kind Kind) (Item, error) {
	it := Item{Path: dir}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil // unreadable: skip
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		it.Bytes += allocated(info)
		if !d.IsDir() && info.ModTime().After(it.LastUsed) { // a folder's time moves whenever a file is added
			it.LastUsed = info.ModTime()
		}
		switch {
		case kind == Temp && d.IsDir() && filepath.Dir(p) == dir:
			it.Sessions++
		case kind == Transcripts && !d.IsDir() && strings.HasSuffix(p, ".jsonl"):
			it.Sessions++
		}
		return nil
	})
	if ctx.Err() != nil {
		return Item{}, ctx.Err()
	}
	return it, err
}

func allocated(info fs.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks) * 512
	}
	return info.Size()
}

// projectLabel turns Claude Code's folder name for a project back into its path.
// Claude Code writes the path with every "/" and "." replaced by "-", which loses
// information, so the path is rebuilt by trying the candidates that exist on disk.
func projectLabel(slug, home string) string {
	if p := resolveSlug(slug); p != "" {
		return tildify(p, home)
	}
	return slug
}

func resolveSlug(slug string) string {
	if !strings.HasPrefix(slug, "-") {
		return ""
	}
	parts := strings.Split(slug[1:], "-")
	var walk func(dir string, i int) string
	walk = func(dir string, i int) string {
		if i == len(parts) {
			return dir
		}
		// Grow the next component one part at a time; a "-" inside a name can stand
		// for "-" or "."; an empty part means the original had "/." or "..".
		for j := i + 1; j <= len(parts); j++ {
			for _, name := range names(parts[i:j]) {
				if name == "" {
					continue
				}
				next := filepath.Join(dir, name)
				if st, err := os.Stat(next); err == nil && st.IsDir() {
					if p := walk(next, j); p != "" {
						return p
					}
				}
			}
		}
		return ""
	}
	return walk(string(filepath.Separator), 0)
}

// names lists the spellings parts could have had: each joint a "-" or a ".". A
// leading empty part means the name itself began with "." or "-" (".claude").
// Past 64 spellings, later joints are taken as "-", the common case.
func names(parts []string) []string {
	out := []string{parts[0]}
	for _, p := range parts[1:] {
		next := make([]string, 0, 2*len(out))
		for _, prefix := range out {
			next = append(next, prefix+"-"+p)
			if len(out) <= 32 {
				next = append(next, prefix+"."+p)
			}
		}
		out = next
	}
	return out
}

func tildify(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
