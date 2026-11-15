package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/gitx"
)

func TestKeptCountsEachKeptWorktreeOnce(t *testing.T) {
	f := newFixture(t)
	optIn(t, f.repo, `min_age = "0"`)
	p, err := Build(t.Context(), gitx.Exec{}, f.repo, Options{Now: time.Now()})
	require.NoError(t, err)

	items := byPath(p)
	assert.Equal(t, "clean", items[f.dirty].KeptBy)
	assert.Equal(t, "unlocked", items[f.locked].KeptBy)

	kept := Kept([]Plan{p}, nil)
	_, keep := p.Counts()
	total := 0
	for _, k := range kept {
		total += k.Count
	}
	assert.Equal(t, keep+p.ActiveCount, total, "every kept worktree is counted once: %v", kept)
	assert.Contains(t, kept, KeptCount{Reason: "clean", Count: 1})
	assert.Contains(t, kept, KeptCount{Reason: KeptUnmerged, Count: p.ActiveCount})
}

func TestKeptFollowsThePickersSelection(t *testing.T) {
	p := Plan{Items: []Item{
		{Action: Keep, KeptBy: KeptIdle, Optional: true}, // picked
		{Action: Keep, KeptBy: KeptIdle, Optional: true},
		{Action: Remove}, // left out by the person
		{Action: Remove}, // picked
	}}
	kept := Kept([]Plan{p}, func(_, item int) bool { return item == 0 || item == 3 })
	assert.Equal(t, []KeptCount{{KeptIdle, 1}, {KeptUnselected, 1}}, kept)
	assert.Equal(t, "Why the other 2 stay: 1 is idle (pick it in zap to remove it), 1 wasn't selected.", KeptLine(kept))
}

func TestKeptLine(t *testing.T) {
	cases := []struct {
		name string
		in   []KeptCount
		want string
	}{
		{"nothing kept", nil, ""},
		{"one", []KeptCount{{"clean", 1}}, "Why the other one stays: it has uncommitted changes."},
		{"several", []KeptCount{{KeptUnmerged, 3}, {"clean", 2}, {"keep-ignored", 1}},
			"Why the other 6 stay: 3 have unmerged work, 2 have uncommitted changes, 1 holds an ignored file to keep, like .env."},
		{"unknown gate", []KeptCount{{"future-gate", 2}}, "Why the other 2 stay: 2 were kept by the future-gate check."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, KeptLine(c.in)) })
	}
}

func TestKeptOrderMostCommonFirst(t *testing.T) {
	p := Plan{ActiveCount: 1, Items: []Item{
		{Action: Keep, KeptBy: "clean"}, {Action: Keep, KeptBy: "clean"},
		{Action: Keep, KeptBy: "old-enough"},
		{Action: Remove},
	}}
	assert.Equal(t, []KeptCount{{"clean", 2}, {KeptUnmerged, 1}, {"old-enough", 1}}, Kept([]Plan{p}, nil))
}
