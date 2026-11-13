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
	assert.Contains(t, out, "it does not delete them")
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
