package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/plan"
)

func item(path string, action plan.Action, size int64) plan.Item {
	it := plan.Item{Path: path, Branch: strings.TrimPrefix(path, "/r/"), Status: model.StatusMerged, Action: action,
		Size: &model.Usage{Total: size}, DeleteBranch: true}
	if action == plan.Keep {
		it.Refusal = "locked: claude agent running"
	}
	return it
}

func fixture() []plan.Plan {
	return []plan.Plan{
		{Repo: "/home/u/dev/a", OptedIn: true, Target: model.Target{Name: "main"}, Items: []plan.Item{
			item("/home/u/dev/a/w1", plan.Remove, 1_000_000_000),
			item("/home/u/dev/a/w2", plan.Keep, 5),
			item("/home/u/dev/a/w3", plan.Remove, 500_000_000),
		}},
		{Repo: "/home/u/dev/b", OptedIn: false, Target: model.Target{Name: "main"}, Items: []plan.Item{
			{Path: "/home/u/dev/b/w", Status: model.StatusEmpty, Action: plan.Keep, Eligible: true, Refusal: "no .zap"},
		}},
	}
}

func press(t *testing.T, p *Picker, keys ...string) tea.Cmd {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "ctrl+c":
			msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		require.Equal(t, k, msg.String(), "key encoding")
		_, cmd = p.Update(msg)
	}
	return cmd
}

// view returns the picker's screen as plain text.
func view(p *Picker) string { return ansi.Strip(p.View().Content) }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestPreselectionAndToggle(t *testing.T) {
	p := New(fixture(), "/home/u", nil)
	n, size := p.count()
	assert.Equal(t, 2, n, "removable items start selected")
	assert.Equal(t, int64(1_500_000_000), size)
	assert.Contains(t, view(p), "2 selected · 1.5 GB")
	assert.True(t, p.View().AltScreen)

	press(t, p, "space") // cursor starts on w1
	n, _ = p.count()
	assert.Equal(t, 1, n)

	press(t, p, "j", "space") // w2 is kept: cannot be selected
	n, _ = p.count()
	assert.Equal(t, 1, n)
	assert.Contains(t, view(p), "kept: locked: claude agent running")

	press(t, p, "a") // toggle the whole repo on
	n, _ = p.count()
	assert.Equal(t, 2, n)
	press(t, p, "a")
	n, _ = p.count()
	assert.Zero(t, n)
}

func TestNavigationSkipsHeaders(t *testing.T) {
	p := New(fixture(), "/home/u", nil)
	press(t, p, "]")
	assert.Equal(t, 1, p.rows[p.cursor].plan, "] jumps to the next repository's first item")
	press(t, p, "[")
	assert.Equal(t, 0, p.rows[p.cursor].plan)
	press(t, p, "k", "k", "k")
	assert.False(t, p.rows[p.cursor].header)
	press(t, p, "j", "j", "j", "j", "j", "j")
	assert.False(t, p.rows[p.cursor].header)
}

func TestReviewConfirmAndBack(t *testing.T) {
	p := New(fixture(), "/home/u", nil)
	press(t, p, "enter")
	v := view(p)
	assert.Contains(t, v, "Zap 2 worktrees (about 1.5 GB)?")
	assert.Contains(t, v, "deleted for good")
	press(t, p, "n")
	assert.Equal(t, listMode, p.mode)
	assert.False(t, p.Confirmed())

	press(t, p, "enter")
	cmd := press(t, p, "y")
	assert.True(t, p.Confirmed())
	assert.True(t, isQuit(cmd))
}

func TestQuitWithoutConfirming(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		p := New(fixture(), "/home/u", nil)
		assert.True(t, isQuit(press(t, p, k)), k)
		assert.False(t, p.Confirmed(), k)
	}
}

func TestEnterWithNothingSelected(t *testing.T) {
	p := New(fixture(), "/home/u", nil)
	press(t, p, "a") // deselect all in repo a
	press(t, p, "enter")
	assert.Equal(t, listMode, p.mode)
	assert.Contains(t, view(p), "nothing selected")
}

func TestDetail(t *testing.T) {
	pl := fixture()
	pl[0].Items[0].Gates = []model.CheckResult{{Check: "clean", Outcome: model.Pass, Detail: "no uncommitted changes"}}
	p := New(pl, "/home/u", nil)
	press(t, p, "e")
	v := view(p)
	assert.Contains(t, v, "~/dev/a/w1")
	assert.Contains(t, v, "no uncommitted changes")
	assert.Contains(t, v, "remove worktree, delete branch")
	press(t, p, "esc")
	assert.Equal(t, listMode, p.mode)
}

func TestOptIn(t *testing.T) {
	calls := 0
	optIn := func(repo string) (plan.Plan, error) {
		calls++
		if calls > 1 {
			return plan.Plan{}, errors.New("disk full")
		}
		return plan.Plan{Repo: repo, OptedIn: true, Items: []plan.Item{{Path: repo + "/w", Status: model.StatusEmpty, Action: plan.Remove}}}, nil
	}
	p := New(fixture(), "/home/u", optIn)
	press(t, p, "]")
	cmd := press(t, p, "i")
	require.NotNil(t, cmd)
	p.Update(cmd())
	n, _ := p.count()
	assert.Equal(t, 3, n, "the freshly opted-in item is preselected")
	assert.Contains(t, view(p), "opted in ~/dev/b")

	assert.Nil(t, press(t, p, "i"), "already opted in")
}

func TestConstantHeightAndScroll(t *testing.T) {
	var items []plan.Item
	for i := range 50 {
		items = append(items, item("/r/w"+string(rune('a'+i%26))+string(rune('a'+i/26)), plan.Remove, 1))
	}
	p := New([]plan.Plan{{Repo: "/r", OptedIn: true, Items: items}}, "", nil)
	p.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	lines := strings.Count(view(p), "\n")
	for range 40 {
		press(t, p, "j")
	}
	assert.Equal(t, lines, strings.Count(view(p), "\n"), "height does not change while scrolling")
	assert.Contains(t, view(p), "› ", "cursor stays visible")
}

func TestIdleIsOfferedNotPreselected(t *testing.T) {
	pl := fixture()
	pl[0].Items = append(pl[0].Items, plan.Item{Path: "/home/u/dev/a/old", Status: model.StatusIdle,
		Action: plan.Keep, Optional: true, Eligible: true, Refusal: "idle"})
	p := New(pl, "/home/u", nil)
	n, _ := p.count()
	assert.Equal(t, 2, n, "idle starts unselected")
	press(t, p, "j", "j", "j", "space")
	n, _ = p.count()
	assert.Equal(t, 3, n, "idle can be picked by hand")
}
