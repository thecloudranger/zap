package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thecloudranger/zap/internal/leftovers"
)

func TestRenderMap(t *testing.T) {
	r := MapResult{
		Free: 231e6, Total: 1.8e12,
		Worktrees: []MapAgent{
			{Agent: "claude", Bytes: 940e6, Stale: 940e6, Count: 2},
			{Agent: "", Bytes: 218e6, Count: 1},
		},
		Leftovers: []leftovers.Group{
			{Agent: "claude", Kind: leftovers.Temp, Path: "/home/me/tmp/claude-1000", Bytes: 4.6e9, Sessions: 29, LastUsed: time.Now(),
				Items: []leftovers.Item{{Label: "~/dev/api", Bytes: 2.5e9, Sessions: 9}, {Label: "~/dev/web", Bytes: 1.2e9, Sessions: 12},
					{Label: "~/dev/cli", Bytes: 0.5e9, Sessions: 4}, {Label: "~/dev/docs", Bytes: 0.4e9, Sessions: 4}}},
			{Agent: "codex", Kind: leftovers.Transcripts, Bytes: 83e6, Sessions: 140},
		},
	}
	var b bytes.Buffer
	renderMap(&b, r, palette{}, "/home/me", 3)
	out := b.String()
	assert.Contains(t, out, "Coding agents keep 5.8 GB on this disk. It has 231 MB free of 1.8 TB.")
	assert.Contains(t, out, "▒ merged or empty worktrees 940 MB")
	assert.Contains(t, out, "▓ temp sessions 4.6 GB")
	assert.Contains(t, out, "29 sessions in ~/tmp/claude-1000")
	assert.Contains(t, out, "1 more project")
	assert.Contains(t, out, "needed to resume them")
	assert.Contains(t, out, "zap leaves temp sessions and transcripts on disk.")
	assert.NotContains(t, out, "\x1b[", "no colour without a terminal")

	lines := strings.Split(out, "\n")
	bar := []rune(strings.TrimSpace(lines[2]))
	assert.Len(t, bar, 72, "the stacked bar fills its width")

	for _, parts := range [][]int64{{1, 1, 1, 1}, {1e9, 1, 1, 1}, {3, 3, 3, 0}, {0, 0, 0, 5}, {7, 0, 0, 1}} {
		got := []rune(stacked(palette{}, 72, parts, []rune{fillStale, fillUsed, fillTemp, fillLog}))
		assert.Len(t, got, 72, "%v", parts)
	}

	var empty bytes.Buffer
	renderMap(&empty, MapResult{}, palette{}, "/home/me", 3)
	assert.Contains(t, empty.String(), "No agent worktrees")
}

func TestRenderTree(t *testing.T) {
	r := MapResult{
		Free: 13e9, Total: 494e9,
		Worktrees: []MapAgent{
			{Agent: "claude", Bytes: 940e6, Stale: 940e6, Items: []MapWorktree{{Label: "auth", Bytes: 537e6, Stale: true}, {Label: "fix-login", Bytes: 403e6, Stale: true}}},
			{Agent: "", Bytes: 218e6, Items: []MapWorktree{{Label: "search", Bytes: 218e6}}},
		},
		Leftovers: []leftovers.Group{
			{Agent: "claude", Kind: leftovers.Temp, Bytes: 600e6, Items: []leftovers.Item{{Label: "~/dev/my-app", Bytes: 400e6}, {Label: "~/dev/api", Bytes: 200e6}}},
			{Agent: "codex", Kind: leftovers.Transcripts, Bytes: 90e6},
		},
	}
	var b bytes.Buffer
	renderTree(&b, r, palette{}, 100, 20)
	out := b.String()
	assert.Contains(t, out, "Coding agents keep 1.8 GB on this disk. It has 13 GB free of 494 GB.")
	for _, s := range []string{"Claude Code worktrees", "auth", "537 MB", "fix-login", "search", "my-app", "Codex"} {
		assert.Contains(t, out, s)
	}
	assert.Contains(t, out, "▒▒ merged or empty worktrees 940 MB")
	assert.NotContains(t, out, "\x1b[")
	assert.NotContains(t, out, "… MB", "sizes are never cut")
	assert.NotContains(t, out, "~/dev/my-app", "tree blocks show the project folder, not its path")

	lines := strings.Split(out, "\n")
	grid := lines[2 : 2+20]
	for _, l := range grid {
		assert.LessOrEqual(t, len([]rune(l)), 2+96, "the drawing stays inside the terminal: %q", l)
	}
	assert.Contains(t, grid[1], "▒", "merged worktrees are hatched")
	assert.Contains(t, out, "█", "worktrees in use are solid")

	assert.Equal(t, "fix-lo…", fit("fix-login", 7, true))
	assert.Equal(t, "…/my-app", fit("~/dev/my-app", 8, false))
}

func TestFitPath(t *testing.T) {
	long := "~/code/llm-eval-harness-with-structured-output-graders"
	for in, want := range map[string]string{
		"~/dev/my-app":                "~/dev/my-app",
		"~/dev/clients/northwind/web": "~/dev/clients/northwind/web",
		"~/dev/clients/northwind/platform/services/payments-reconciliation-service": "…/payments-reconciliation-service",
		"~/dev/clients/northwind-traders/platform/billing/ui":                       "…/platform/billing/ui",
		long: "…" + long[len(long)-33:],
	} {
		got := fitPath(in, 34)
		assert.Equal(t, want, got, in)
		assert.LessOrEqual(t, len([]rune(got)), 34, in)
	}
}
