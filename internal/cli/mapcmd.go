package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/leftovers"
	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/sizer"
)

// MapResult is the `data` payload of `zap map --json`.
type MapResult struct {
	Free      uint64            `json:"freeBytes"`
	Total     uint64            `json:"totalBytes"`
	Worktrees []MapAgent        `json:"worktrees"` // linked worktrees per agent
	Leftovers []leftovers.Group `json:"leftovers"`
}

// MapAgent is one agent's linked worktrees: all of them, and the stale (merged or
// empty) part that `zap clean` can remove in opted-in repositories.
type MapAgent struct {
	Agent string        `json:"agent"`
	Bytes int64         `json:"bytes"`
	Stale int64         `json:"staleBytes"`
	Count int           `json:"count"`
	Items []MapWorktree `json:"items"`
}

// MapWorktree is one linked worktree on the map.
type MapWorktree struct {
	Label string `json:"label"` // short name: the folder, with its parent when that says more
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Stale bool   `json:"stale"` // merged or empty
}

// The fills say what a block is; colour only repeats it, so the picture reads without colour.
const (
	fillStale = '▒' // stale worktrees: zap clean frees this
	fillUsed  = '█' // worktrees in use
	fillTemp  = '▓' // agent temp sessions
	fillLog   = '░' // transcripts, needed to resume sessions
)

var agentNames = map[string]string{
	"claude": "Claude Code", "codex": "Codex", "cursor": "Cursor", "conductor": "Conductor", "t3": "T3 Code", "": "other",
}

func newMapCmd(d Deps) *cobra.Command {
	var asJSON, tree, list bool
	var top, height int
	cmd := &cobra.Command{
		Use:   "map",
		Short: "Draw what coding agents keep on this disk: worktrees, temp sessions, transcripts",
		Long: "Scan every repository under the scan roots and the folders where agents keep their\n" +
			"temp sessions and transcripts, then draw it to scale. Nothing is deleted.\n\n" +
			"It lists agents and projects with bars; --tree (experimental) draws nested blocks sized\n" +
			"by disk use, like a disk treemap.\n\n" +
			"  ▒ merged or empty worktrees, which zap reviews for removal\n" +
			"  █ worktrees in use\n" +
			"  ▓ agent temp sessions\n" +
			"  ░ transcripts, which agents need to resume a session",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			res, err := buildMap(cmd, d, asJSON)
			if asJSON {
				if err != nil {
					return writeErr(out, "map", err)
				}
				return writeJSON(out, Envelope{Command: "map", Status: "ok", Data: res})
			}
			if err != nil {
				return err
			}
			if tree && list {
				return usageErr(errors.New("--tree and --list cannot be used together"))
			}
			if tree { // experimental: the list is the default while the tree view is refined
				renderTree(out, res, mapStyle(d, out), termCols(out), height)
				return nil
			}
			renderMap(out, res, mapStyle(d, out), d.Home, top)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	cmd.Flags().IntVar(&top, "top", 3, "projects to list under each kind of agent data")
	cmd.Flags().BoolVar(&tree, "tree", false, "experimental: draw nested blocks sized by disk use, like a disk treemap")
	cmd.Flags().BoolVar(&list, "list", false, "list agents and projects with bars (the default)")
	cmd.Flags().IntVar(&height, "height", 22, "rows the --tree drawing uses")
	return cmd
}

func buildMap(cmd *cobra.Command, d Deps, quiet bool) (MapResult, error) {
	scanned, err := scanAll(cmd, d, inspect.Options{Sizer: sizer.NewCounter()}, quiet)
	if err != nil {
		return MapResult{}, err
	}
	res := MapResult{Free: scanned.Free, Total: scanned.Total, Worktrees: []MapAgent{}, Leftovers: []leftovers.Group{}}
	by := map[string]*MapAgent{}
	for _, rep := range scanned.Repos {
		for _, a := range rep.Worktrees {
			if a.Main || a.Facts.Size == nil {
				continue
			}
			m := by[a.Agent]
			if m == nil {
				m = &MapAgent{Agent: a.Agent}
				by[a.Agent] = m
			}
			m.Count++
			m.Bytes += a.Facts.Size.Total
			stale := a.Verdict.Status == model.StatusMerged || a.Verdict.Status == model.StatusEmpty
			if stale {
				m.Stale += a.Facts.Size.Total
			}
			m.Items = append(m.Items, MapWorktree{Label: shortName(a.Path, rep.Root), Path: a.Path, Bytes: a.Facts.Size.Total, Stale: stale})
		}
	}
	for _, m := range by {
		res.Worktrees = append(res.Worktrees, *m)
	}
	sort.Slice(res.Worktrees, func(i, j int) bool { return res.Worktrees[i].Bytes > res.Worktrees[j].Bytes })
	gs, err := leftovers.Find(cmd.Context(), leftovers.Options{Home: d.Home, TmpRoot: d.TmpRoot})
	if err != nil {
		return MapResult{}, err
	}
	if gs != nil {
		res.Leftovers = gs
	}
	return res, nil
}

// palette paints a fill; the zero value paints nothing.
type palette struct {
	on    bool
	fills map[rune]lipgloss.Style
	dim   lipgloss.Style
	bold  lipgloss.Style
}

func mapStyle(d Deps, w io.Writer) palette {
	f, isFile := w.(*os.File)
	if os.Getenv("NO_COLOR") != "" || !isFile || !term.IsTerminal(int(f.Fd())) || d.IsTTY == nil || !d.IsTTY() {
		return palette{}
	}
	c := lipgloss.Color
	return palette{on: true,
		fills: map[rune]lipgloss.Style{
			fillStale: lipgloss.NewStyle().Foreground(c("#FFD21F")),
			fillUsed:  lipgloss.NewStyle().Foreground(c("#89B4FA")),
			fillTemp:  lipgloss.NewStyle().Foreground(c("#F38BA8")),
			fillLog:   lipgloss.NewStyle().Foreground(c("#A6ADC8")),
		},
		dim:  lipgloss.NewStyle().Foreground(c("#7F849C")),
		bold: lipgloss.NewStyle().Bold(true),
	}
}

func (p palette) fill(r rune, n int) string {
	s := strings.Repeat(string(r), n)
	if p.on && n > 0 {
		return p.fills[r].Render(s)
	}
	return s
}

func (p palette) faint(s string) string {
	if p.on {
		return p.dim.Render(s)
	}
	return s
}

func (p palette) strong(s string) string {
	if p.on {
		return p.bold.Render(s)
	}
	return s
}

// row is one line of the map: a label, a size, a bar of one or two fills, a note.
type row struct {
	indent   int
	label    string
	bytes    int64
	part     int64 // drawn with partFill at the start of the bar
	fill     rune
	partFill rune
	note     string
	heading  bool
}

func renderMap(w io.Writer, r MapResult, p palette, home string, top int) {
	var rows []row
	var stale, used, temp, logs int64
	if len(r.Worktrees) > 0 {
		var all, st int64
		for _, a := range r.Worktrees {
			all += a.Bytes
			st += a.Stale
		}
		stale, used = st, all-st
		note := "none merged or empty"
		if st > 0 {
			note = sizer.Human(st) + " merged or empty"
		}
		rows = append(rows, row{label: "Worktrees", bytes: all, part: st, fill: fillUsed, partFill: fillStale, note: note, heading: true})
		for _, a := range r.Worktrees {
			n := fmt.Sprintf("%d worktree%s", a.Count, plural(a.Count))
			if a.Stale > 0 {
				n += ", " + sizer.Human(a.Stale) + " merged or empty"
			}
			rows = append(rows, row{indent: 1, label: agentNames[a.Agent], bytes: a.Bytes, part: a.Stale, fill: fillUsed, partFill: fillStale, note: n})
		}
	}
	for _, g := range r.Leftovers {
		fill, title := fillLog, agentNames[g.Agent]+" transcripts"
		if g.Kind == leftovers.Temp {
			fill, title = fillTemp, agentNames[g.Agent]+" temp sessions"
			temp += g.Bytes
		} else {
			logs += g.Bytes
		}
		note := fmt.Sprintf("%d session%s", g.Sessions, plural(g.Sessions))
		if g.Kind == leftovers.Transcripts {
			note += ", needed to resume them"
		} else {
			note += " in " + shorten(tildify(g.Path, home), 40)
		}
		rows = append(rows, row{label: title, bytes: g.Bytes, fill: fill, note: note, heading: true})
		shown := 0
		var rest int64
		for _, it := range g.Items {
			if shown < top {
				rows = append(rows, row{indent: 1, label: it.Label, bytes: it.Bytes, fill: fill,
					note: fmt.Sprintf("%d session%s", it.Sessions, plural(it.Sessions))})
				shown++
			} else {
				rest += it.Bytes
			}
		}
		if more := len(g.Items) - shown; more > 0 {
			rows = append(rows, row{indent: 1, label: fmt.Sprintf("%d more project%s", more, plural(more)), bytes: rest, fill: fill})
		}
	}

	all := stale + used + temp + logs
	if all == 0 {
		_, _ = fmt.Fprintln(w, "No agent worktrees, temp sessions or transcripts found.")
		return
	}
	cols := termCols(w)

	// The headline: all agent data as one bar, and the disk it sits on.
	head := "Coding agents keep " + sizer.Human(all) + " on this disk"
	if r.Total > 0 {
		head += fmt.Sprintf(". It has %s free of %s.", sizer.Human(int64(r.Free)), sizer.Human(int64(r.Total)))
	}
	_, _ = fmt.Fprintln(w, p.strong(head))
	_, _ = fmt.Fprintln(w)
	width := min(cols-4, 72)
	_, _ = fmt.Fprintln(w, "  "+stacked(p, width, []int64{stale, used, temp, logs}, []rune{fillStale, fillUsed, fillTemp, fillLog}))
	var legend []string
	for _, k := range []struct {
		r rune
		n int64
		s string
	}{{fillStale, stale, "merged or empty worktrees"}, {fillUsed, used, "worktrees in use"}, {fillTemp, temp, "temp sessions"}, {fillLog, logs, "transcripts"}} {
		if k.n > 0 {
			legend = append(legend, p.fill(k.r, 1)+" "+k.s+" "+sizer.Human(k.n))
		}
	}
	_, _ = fmt.Fprintln(w, "  "+strings.Join(legend, "   "))
	_, _ = fmt.Fprintln(w)

	// The tree: every row on one scale, so bars compare across kinds.
	var biggest int64
	labelW := 0
	for _, x := range rows {
		biggest = max(biggest, x.bytes)
		labelW = max(labelW, len([]rune(x.label))+2*x.indent)
	}
	labelW = min(labelW, 34)
	barW := max(10, min(30, cols-labelW-10-2-28))
	for i, x := range rows {
		if x.heading && i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		indent := strings.Repeat("  ", x.indent)
		label := indent + fitPath(x.label, labelW-len(indent))
		label += strings.Repeat(" ", labelW-len([]rune(label)))
		if x.heading {
			label = p.strong(label)
		}
		size := fmt.Sprintf("%8s", sizer.Human(x.bytes))
		_, _ = fmt.Fprintf(w, "  %s  %s  %s  %s\n", label, size, bar(p, barW, x, biggest), p.faint(x.note))
	}
	if stale > 0 {
		_, _ = fmt.Fprintln(w, "\nRun zap to see which merged and empty worktrees it can delete.")
	}
	if temp+logs > 0 {
		_, _ = fmt.Fprintln(w, "zap leaves temp sessions and transcripts on disk.")
	}
}

// termCols is the terminal's width, or 100 when w is not a terminal.
func termCols(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if c, _, err := term.GetSize(int(f.Fd())); err == nil && c > 40 {
			return c
		}
	}
	return 100
}

// shortName is a worktree's folder name, with its parent when the folder is only the
// repository's name again (~/.codex/worktrees/a1f3/my-app is "a1f3/my-app").
func shortName(path, repoRoot string) string {
	base := filepath.Base(path)
	if base == filepath.Base(repoRoot) {
		return filepath.Base(filepath.Dir(path)) + "/" + base
	}
	return base
}

// fitPath fits a path into n characters by keeping whole trailing folders after "…/"
// (~/dev/clients/acme/payments-service becomes …/payments-service); a last folder
// that is itself too long keeps its end.
func fitPath(p string, n int) string {
	if len([]rune(p)) <= n {
		return p
	}
	parts := strings.Split(p, "/")
	tail := parts[len(parts)-1]
	for i := len(parts) - 2; i >= 0; i-- {
		next := parts[i] + "/" + tail
		if len([]rune(next))+2 > n {
			break
		}
		tail = next
	}
	if len([]rune(tail))+2 <= n && len(parts) > 1 {
		return "…/" + tail
	}
	r := []rune(tail)
	if len(r) <= n {
		return tail
	}
	return "…" + string(r[len(r)-n+1:])
}

// shorten keeps the start and end of s within n characters.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	half := (n - 1) / 2
	return string(r[:half]) + "…" + string(r[len(r)-(n-1-half):])
}

// bar draws x to scale against biggest, padded to width; anything non-zero gets at least one cell.
func bar(p palette, width int, x row, biggest int64) string {
	n := cells(x.bytes, biggest, width)
	part := 0
	if x.part > 0 {
		part = max(1, min(n, cells(x.part, biggest, width)))
	}
	return p.fill(x.partFill, part) + p.fill(x.fill, n-part) + strings.Repeat(" ", width-n)
}

func cells(n, of int64, width int) int {
	if n <= 0 || of <= 0 {
		return 0
	}
	return max(1, int(float64(n)/float64(of)*float64(width)+0.5))
}

// stacked splits width between parts in proportion (largest remainder), each
// non-zero part at least one cell, so the bar is always exactly width cells.
func stacked(p palette, width int, parts []int64, fills []rune) string {
	var total int64
	for _, n := range parts {
		total += n
	}
	if total <= 0 {
		return ""
	}
	c := make([]int, len(parts))
	rem := make([]float64, len(parts))
	used := 0
	for i, n := range parts {
		if n <= 0 {
			continue
		}
		exact := float64(n) / float64(total) * float64(width)
		c[i] = max(1, int(exact))
		rem[i] = exact - float64(int(exact))
		used += c[i]
	}
	for used < width { // the floors leave at most one cell per part: give them to the largest remainders
		best := -1
		for i := range parts {
			if parts[i] > 0 && rem[i] >= 0 && (best < 0 || rem[i] > rem[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		c[best]++
		rem[best] = -1
		used++
	}
	for used > width { // minimum cells overshot: take from the largest part
		big := 0
		for i := range c {
			if c[i] > c[big] {
				big = i
			}
		}
		c[big]--
		used--
	}
	var b strings.Builder
	for i := range parts {
		b.WriteString(p.fill(fills[i], c[i]))
	}
	return b.String()
}
