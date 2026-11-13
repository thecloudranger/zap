package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/thecloudranger/zap/internal/leftovers"
	"github.com/thecloudranger/zap/internal/sizer"
	"github.com/thecloudranger/zap/internal/treemap"
)

// Kinds of block on the tree map. Each has a fill that reads without colour.
const (
	tkGround = iota // outside every block
	tkGroup         // an agent's frame, behind its blocks
	tkStale         // merged or empty worktree
	tkUsed          // worktree in use
	tkTemp          // temp sessions
	tkLog           // transcripts
)

var treeFill = map[int]rune{tkGround: ' ', tkGroup: ' ', tkStale: fillStale, tkUsed: fillUsed, tkTemp: fillTemp, tkLog: fillLog}

type tcell struct {
	r    rune
	kind int
	text bool // a label character, drawn over the block's fill
	bold bool
}

// renderTree draws the map as nested blocks sized by bytes, the way disk treemaps do:
// one frame per agent and kind of data, one block per worktree or project inside it.
func renderTree(w io.Writer, r MapResult, p palette, cols, height int) {
	var top []*treemap.Node
	short := map[*treemap.Node][]string{} // shorter titles, tried in order when a frame is narrow
	var stale, used, temp, logs int64
	for _, a := range r.Worktrees {
		g := &treemap.Node{Label: agentNames[a.Agent] + " worktrees", Weight: a.Bytes, Kind: tkGroup}
		short[g] = []string{agentNames[a.Agent], "worktrees"}
		for _, it := range a.Items {
			k := tkUsed
			if it.Stale {
				k, stale = tkStale, stale+it.Bytes
			} else {
				used += it.Bytes
			}
			g.Children = append(g.Children, &treemap.Node{Label: it.Label, Weight: it.Bytes, Kind: k})
		}
		top = append(top, g)
	}
	for _, lg := range r.Leftovers {
		k, title := tkLog, agentNames[lg.Agent]+" transcripts"
		if lg.Kind == leftovers.Temp {
			k, title = tkTemp, agentNames[lg.Agent]+" temp sessions"
			temp += lg.Bytes
		} else {
			logs += lg.Bytes
		}
		g := &treemap.Node{Label: title, Weight: lg.Bytes, Kind: tkGroup}
		if k == tkTemp {
			short[g] = []string{agentNames[lg.Agent] + " temp", "temp sessions", "temp"}
		} else {
			short[g] = []string{agentNames[lg.Agent] + " logs", "transcripts", "logs"}
		}
		for _, it := range lg.Items {
			// The frame already says what this is; the block names the project, not its path.
			g.Children = append(g.Children, &treemap.Node{Label: filepath.Base(it.Label), Weight: it.Bytes, Kind: k})
		}
		if len(g.Children) == 0 {
			g.Kind = k // nothing inside to frame: draw it as one block
		}
		top = append(top, g)
	}
	all := stale + used + temp + logs
	if all == 0 {
		_, _ = fmt.Fprintln(w, "No agent worktrees, temp sessions or transcripts found.")
		return
	}

	width := max(40, min(cols-4, 140))
	grid := make([][]tcell, height)
	for y := range grid {
		grid[y] = make([]tcell, width)
	}
	treemap.Layout(top, treemap.Rect{W: width, H: height})
	for _, g := range top {
		if len(g.Children) == 0 || g.Children[0].Rect.W == 0 {
			drawBlock(grid, g.Rect, blockKind(g), g.Label, g.Weight, true)
			continue
		}
		fillRect(grid, g.Rect, tkGroup)
		title(grid, g.Rect.X+1, g.Rect.Y, g.Rect.X+g.Rect.W-1, append([]string{g.Label}, short[g]...), sizer.Human(g.Weight))
		for _, c := range g.Children {
			drawBlock(grid, c.Rect, c.Kind, c.Label, c.Weight, false)
		}
	}

	head := "Coding agents keep " + sizer.Human(all) + " on this disk"
	if r.Total > 0 {
		head += fmt.Sprintf(". It has %s free of %s.", sizer.Human(int64(r.Free)), sizer.Human(int64(r.Total)))
	}
	_, _ = fmt.Fprintln(w, p.strong(head))
	_, _ = fmt.Fprintln(w)
	styles := treeStyles(p)
	for _, row := range grid {
		_, _ = fmt.Fprintln(w, "  "+paintRow(row, styles))
	}
	var legend []string
	for _, k := range []struct {
		kind int
		n    int64
		s    string
	}{{tkStale, stale, "merged or empty worktrees"}, {tkUsed, used, "worktrees in use"}, {tkTemp, temp, "temp sessions"}, {tkLog, logs, "transcripts"}} {
		if k.n > 0 {
			legend = append(legend, paintRow([]tcell{{r: treeFill[k.kind], kind: k.kind}, {r: treeFill[k.kind], kind: k.kind}}, styles)+" "+k.s+" "+sizer.Human(k.n))
		}
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "  "+strings.Join(legend, "    "))
	if stale > 0 {
		_, _ = fmt.Fprintln(w, "\nRun zap to see which merged and empty worktrees it can delete.")
	}
	if temp+logs > 0 {
		_, _ = fmt.Fprintln(w, "zap leaves temp sessions and transcripts on disk.")
	}
}

// blockKind is the fill for a frame too small to show what is inside: its largest child's.
func blockKind(g *treemap.Node) int {
	if len(g.Children) == 0 {
		return g.Kind
	}
	best := g.Children[0]
	for _, c := range g.Children {
		if c.Weight > best.Weight {
			best = c
		}
	}
	return best.Kind
}

// drawBlock fills r, leaving its last column and row as a gap so neighbours read as
// separate tiles, and writes the name and size inside when they fit. A worktree or
// project name keeps its end when cut (the telling part of a path); a frame drawn
// as one block keeps its start (the agent's name). A size is never cut: it is
// written whole or not at all.
func drawBlock(grid [][]tcell, r treemap.Rect, kind int, name string, n int64, keepStart bool) {
	in := r
	if in.W > 2 {
		in.W--
	}
	if in.H > 1 {
		in.H--
	}
	fillRect(grid, in, kind)
	room := in.W - 2
	if room < 3 {
		return
	}
	size := sizer.Human(n)
	name = fit(name, room, keepStart)
	switch {
	case in.H >= 2:
		label(grid, in.X+1, in.Y, in.X+in.W, name, false)
		if len(size) <= room {
			label(grid, in.X+1, in.Y+1, in.X+in.W, size, false)
		}
	case len([]rune(name))+1+len(size) <= room:
		label(grid, in.X+1, in.Y, in.X+in.W, name+" "+size, false)
	default:
		label(grid, in.X+1, in.Y, in.X+in.W, name, false)
	}
}

// fit cuts s to room characters with an ellipsis at the end or the start.
func fit(s string, room int, keepStart bool) string {
	rs := []rune(s)
	if len(rs) <= room {
		return s
	}
	if keepStart {
		return string(rs[:room-1]) + "…"
	}
	return "…" + string(rs[len(rs)-room+1:])
}

// title writes a frame's heading and size: the first name that fits whole, else the
// last one cut short, else the size alone.
func title(grid [][]tcell, x, y, right int, names []string, size string) {
	room := right - x - 1 - len(size) - 2
	if room < 3 {
		label(grid, x, y, right, size, true)
		return
	}
	pick := fit(names[len(names)-1], room, true)
	for _, n := range names {
		if len([]rune(n)) <= room {
			pick = n
			break
		}
	}
	label(grid, x, y, right, pick+"  "+size, true)
}

func fillRect(grid [][]tcell, r treemap.Rect, kind int) {
	for y := r.Y; y < r.Y+r.H && y < len(grid); y++ {
		for x := r.X; x < r.X+r.W && x < len(grid[y]); x++ {
			grid[y][x] = tcell{r: treeFill[kind], kind: kind}
		}
	}
}

// label writes s from x on row y, stopping one cell short of right; a name that does
// not fit keeps its end, which is the more telling part of a path.
func label(grid [][]tcell, x, y, right int, s string, bold bool) {
	if y >= len(grid) {
		return
	}
	room := right - x - 1
	rs := []rune(s)
	if room <= 0 {
		return
	}
	if len(rs) > room {
		if room < 3 {
			return
		}
		rs = append([]rune{'…'}, rs[len(rs)-room+1:]...)
	}
	for i, c := range rs {
		if x+i < len(grid[y]) {
			grid[y][x+i] = tcell{r: c, kind: grid[y][x+i].kind, text: true, bold: bold}
		}
	}
}

type tstyle struct{ fill, text lipgloss.Style }

func treeStyles(p palette) map[int]tstyle {
	if !p.on {
		return nil
	}
	c := lipgloss.Color
	mk := func(bg, fillFg, textFg string) tstyle {
		return tstyle{
			fill: lipgloss.NewStyle().Background(c(bg)).Foreground(c(fillFg)),
			text: lipgloss.NewStyle().Background(c(bg)).Foreground(c(textFg)),
		}
	}
	return map[int]tstyle{
		tkGroup: mk("#262637", "#262637", "#F4F1E8"),
		tkStale: mk("#FFD21F", "#C9A20C", "#15151E"),
		tkUsed:  mk("#89B4FA", "#89B4FA", "#15151E"),
		tkTemp:  mk("#F38BA8", "#F38BA8", "#15151E"),
		tkLog:   mk("#585B70", "#585B70", "#F4F1E8"),
	}
}

// paintRow joins a row's cells into runs of one style each.
func paintRow(row []tcell, styles map[int]tstyle) string {
	if styles == nil {
		var b strings.Builder
		for _, c := range row {
			b.WriteRune(c.r)
		}
		return strings.TrimRight(b.String(), " ")
	}
	var b strings.Builder
	for i := 0; i < len(row); {
		j := i
		for j < len(row) && row[j].kind == row[i].kind && row[j].text == row[i].text && row[j].bold == row[i].bold {
			j++
		}
		var run strings.Builder
		for _, c := range row[i:j] {
			r := c.r
			if !c.text && row[i].kind != tkStale {
				r = ' ' // colour carries solid blocks; only the hatch keeps its texture
			}
			run.WriteRune(r)
		}
		st, ok := styles[row[i].kind]
		switch {
		case !ok:
			b.WriteString(run.String())
		case row[i].text:
			s := st.text
			if row[i].bold {
				s = s.Bold(true)
			}
			b.WriteString(s.Render(run.String()))
		default:
			b.WriteString(st.fill.Render(run.String()))
		}
		i = j
	}
	return b.String()
}
