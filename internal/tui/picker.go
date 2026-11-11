// Package tui is zap's interactive picker. It only chooses: it shows the plans, lets the
// user change the selection, and reports what was confirmed. Removal happens elsewhere,
// through the same plan/apply path as `zap clean`.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/thecloudranger/zap/internal/model"
	"github.com/thecloudranger/zap/internal/plan"
	"github.com/thecloudranger/zap/internal/sizer"
)

type mode int

const (
	listMode mode = iota
	detailMode
	reviewMode
)

// OptInFunc opts a repository in and returns its fresh plan.
type OptInFunc func(repo string) (plan.Plan, error)

// Key identifies an item: plan index and item index.
type Key struct{ Plan, Item int }

type row struct {
	header bool
	plan   int
	item   int
}

// Picker is the Bubble Tea model.
type Picker struct {
	plans     []plan.Plan
	rows      []row
	selected  map[Key]bool
	cursor    int // index into rows; always an item row when any exist
	offset    int // first visible body row
	mode      mode
	width     int
	height    int
	home      string
	optIn     OptInFunc
	message   string
	confirmed bool
}

// New builds a picker. Items zap would remove are preselected; kept items cannot be selected.
func New(plans []plan.Plan, home string, optIn OptInFunc) *Picker {
	p := &Picker{plans: plans, home: home, optIn: optIn, selected: map[Key]bool{}, height: 24, width: 100}
	for pi, pl := range plans {
		p.preselect(pi, pl)
	}
	p.rebuild()
	return p
}

// Confirmed reports whether the user confirmed the review screen.
func (p *Picker) Confirmed() bool { return p.confirmed }

// Selected returns the confirmed selection.
func (p *Picker) Selected() map[Key]bool { return p.selected }

// Plans returns the plans, including any refreshed by opting a repository in.
func (p *Picker) Plans() []plan.Plan { return p.plans }

func (p *Picker) preselect(pi int, pl plan.Plan) {
	for ii, it := range pl.Items {
		if it.Action != plan.Keep {
			p.selected[Key{pi, ii}] = true
		} else {
			delete(p.selected, Key{pi, ii})
		}
	}
}

func (p *Picker) rebuild() {
	p.rows = p.rows[:0]
	for pi, pl := range p.plans {
		if len(pl.Items) == 0 {
			continue
		}
		p.rows = append(p.rows, row{header: true, plan: pi})
		for ii := range pl.Items {
			p.rows = append(p.rows, row{plan: pi, item: ii})
		}
	}
	if p.cursor >= len(p.rows) || p.cursor < 0 || (len(p.rows) > 0 && p.rows[p.cursor].header) {
		p.cursor = 0
		p.move(1)
	}
}

func (p *Picker) item(r row) plan.Item { return p.plans[r.plan].Items[r.item] }

func (p *Picker) selectable(r row) bool { return !r.header && p.item(r).Action != plan.Keep }

// Init implements tea.Model.
func (p *Picker) Init() tea.Cmd { return nil }

type optedInMsg struct {
	plan int
	pl   plan.Plan
	err  error
}

// Update implements tea.Model.
func (p *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width, p.height = msg.Width, msg.Height
		p.scroll()
	case optedInMsg:
		if msg.err != nil {
			p.message = "opt in failed: " + msg.err.Error()
			break
		}
		p.plans[msg.plan] = msg.pl
		p.preselect(msg.plan, msg.pl)
		p.rebuild()
		p.message = "opted in " + p.short(msg.pl.Repo) + " (wrote .zap)"
	case tea.KeyPressMsg:
		return p.key(msg.String())
	}
	return p, nil
}

func (p *Picker) key(k string) (tea.Model, tea.Cmd) {
	if k == "ctrl+c" {
		return p, tea.Quit
	}
	switch p.mode {
	case detailMode:
		if k == "esc" || k == "e" || k == "q" || k == "enter" {
			p.mode = listMode
		}
		return p, nil
	case reviewMode:
		switch k {
		case "y":
			p.confirmed = true
			return p, tea.Quit
		case "n", "esc", "q":
			p.mode = listMode
		}
		return p, nil
	}
	p.message = ""
	switch k {
	case "q", "esc":
		return p, tea.Quit
	case "down", "j":
		p.move(1)
	case "up", "k":
		p.move(-1)
	case "]":
		p.jumpRepo(1)
	case "[":
		p.jumpRepo(-1)
	case "space", "x":
		p.toggle()
	case "a":
		p.toggleRepo()
	case "e", "right", "l":
		if len(p.rows) > 0 {
			p.mode = detailMode
		}
	case "i":
		return p, p.optInCurrent()
	case "enter":
		if n, _ := p.count(); n > 0 {
			p.mode = reviewMode
		} else {
			p.message = "nothing selected"
		}
	}
	p.scroll()
	return p, nil
}

// move steps the cursor to the next item row in direction dir.
func (p *Picker) move(dir int) {
	for i := p.cursor + dir; i >= 0 && i < len(p.rows); i += dir {
		if !p.rows[i].header {
			p.cursor = i
			return
		}
	}
}

func (p *Picker) jumpRepo(dir int) {
	if len(p.rows) == 0 {
		return
	}
	cur := p.rows[p.cursor].plan
	for i := p.cursor + dir; i >= 0 && i < len(p.rows); i += dir {
		if p.rows[i].header && p.rows[i].plan != cur {
			p.cursor = i
			p.move(1) // land on the repository's first item
			return
		}
	}
}

func (p *Picker) toggle() {
	if len(p.rows) == 0 {
		return
	}
	r := p.rows[p.cursor]
	if !p.selectable(r) {
		p.message = "kept: " + p.item(r).Refusal
		return
	}
	k := Key{r.plan, r.item}
	p.selected[k] = !p.selected[k]
}

func (p *Picker) toggleRepo() {
	if len(p.rows) == 0 {
		return
	}
	pi := p.rows[p.cursor].plan
	all := true
	for _, r := range p.rows {
		if r.plan == pi && p.selectable(r) && !p.selected[Key{r.plan, r.item}] {
			all = false
		}
	}
	for _, r := range p.rows {
		if r.plan == pi && p.selectable(r) {
			p.selected[Key{r.plan, r.item}] = !all
		}
	}
}

func (p *Picker) optInCurrent() tea.Cmd {
	if len(p.rows) == 0 || p.optIn == nil {
		return nil
	}
	pi := p.rows[p.cursor].plan
	if p.plans[pi].OptedIn {
		p.message = "already opted in"
		return nil
	}
	repo, optIn := p.plans[pi].Repo, p.optIn
	return func() tea.Msg {
		pl, err := optIn(repo)
		return optedInMsg{plan: pi, pl: pl, err: err}
	}
}

// count returns the number and measured size of selected items.
func (p *Picker) count() (int, int64) {
	n, size := 0, int64(0)
	for k, on := range p.selected {
		if !on {
			continue
		}
		n++
		if s := p.plans[k.Plan].Items[k.Item].Size; s != nil {
			size += s.Total
		}
	}
	return n, size
}

// bodyHeight is the number of list rows that fit between the fixed header and footer.
func (p *Picker) bodyHeight() int { return max(3, p.height-5) }

func (p *Picker) scroll() {
	h := p.bodyHeight()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+h {
		p.offset = p.cursor - h + 1
	}
	if p.offset > 0 && p.rows[p.offset-1].header && p.cursor-p.offset+1 < h {
		p.offset-- // keep the repository header visible above its first item
	}
}

// View implements tea.Model. It is pure: no I/O, time or randomness.
func (p *Picker) View() tea.View {
	var b strings.Builder
	n, size := p.count()
	b.WriteString(titleStyle.Render("⚡ zap") + "  " + fmt.Sprintf("%d selected", n))
	if size > 0 {
		b.WriteString(" · " + sizeStyle.Render(sizer.Human(size)))
	}
	b.WriteString("\n\n")
	switch p.mode {
	case detailMode:
		b.WriteString(p.detail())
	case reviewMode:
		b.WriteString(p.review())
	default:
		b.WriteString(p.list())
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "zap"
	return v
}

func (p *Picker) list() string {
	var b strings.Builder
	h := p.bodyHeight()
	if len(p.rows) == 0 {
		b.WriteString("Nothing to zap: no merged, empty or missing worktrees.\n")
	}
	end := min(len(p.rows), p.offset+h)
	for i := p.offset; i < end; i++ {
		b.WriteString(p.renderRow(i) + "\n")
	}
	for i := end - p.offset; i < h; i++ {
		b.WriteString("\n") // constant height: no resize jitter
	}
	b.WriteString("\n")
	if p.message != "" {
		b.WriteString(warnStyle.Render(p.message) + "\n")
	} else {
		b.WriteString(helpStyle.Render("↑/↓ move · space toggle · a toggle repo · [ ] repo · e details · i opt in · enter review · q quit") + "\n")
	}
	return b.String()
}

func (p *Picker) renderRow(i int) string {
	r := p.rows[i]
	pl := p.plans[r.plan]
	if r.header {
		s := repoStyle.Render(p.short(pl.Repo)) + helpStyle.Render("  target "+pl.Target.Name)
		if !pl.OptedIn {
			s += "  " + optOutStyle.Render("not opted in: press i to opt in")
		}
		return s
	}
	it := pl.Items[r.item]
	cursor := "  "
	if i == p.cursor {
		cursor = cursorStyle.Render("› ")
	}
	box := "[ ]"
	if p.selected[Key{r.plan, r.item}] {
		box = selStyle.Render("[x]")
	}
	size := "-"
	if it.Size != nil {
		size = sizer.Human(it.Size.Total)
	}
	status := string(it.Status)
	if st, ok := statusStyles[status]; ok {
		status = st.Render(fmt.Sprintf("%-7s", status))
	}
	if it.Action == plan.Keep {
		return cursor + keepStyle.Render(fmt.Sprintf(" ·  %-34s %-22s %-7s %8s  %s", p.rel(it.Path, pl.Repo), clip(dash(it.Branch), 22), it.Status, size, clip(it.Refusal, 50)))
	}
	return fmt.Sprintf("%s%s %-34s %-22s %s %8s", cursor, box, p.rel(it.Path, pl.Repo), clip(dash(it.Branch), 22), status, size)
}

func (p *Picker) detail() string {
	if len(p.rows) == 0 {
		return ""
	}
	r := p.rows[p.cursor]
	pl, it := p.plans[r.plan], p.item(r)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", emphStyle.Render(p.short(it.Path)))
	fmt.Fprintf(&b, "  repo     %s\n", p.short(pl.Repo))
	fmt.Fprintf(&b, "  branch   %s @ %s\n", dash(it.Branch), short(it.Head))
	fmt.Fprintf(&b, "  status   %s: %s\n", it.Status, it.Reason)
	if it.Size != nil {
		fmt.Fprintf(&b, "  size     %s freed (%s dependencies)", sizer.Human(it.Size.Total), sizer.Human(it.Size.Deps))
		if it.Size.Shared > 0 {
			fmt.Fprintf(&b, ", %s shared not freed", sizer.Human(it.Size.Shared))
		}
		b.WriteString("\n")
	}
	if it.Action == plan.Keep {
		fmt.Fprintf(&b, "  kept     %s\n", it.Refusal)
	} else {
		fmt.Fprintf(&b, "  action   %s\n", it.Describe())
	}
	b.WriteString("\n  safety checks\n")
	for _, g := range it.Gates {
		mark := passStyle.Render("✓")
		switch g.Outcome {
		case model.Fail, model.Errored:
			mark = failStyle.Render("✗")
		case model.Skipped:
			mark = helpStyle.Render("-")
		}
		fmt.Fprintf(&b, "    %s %-12s %s\n", mark, g.Check, g.Detail)
	}
	b.WriteString("\n" + helpStyle.Render("esc back") + "\n")
	return b.String()
}

func (p *Picker) review() string {
	var b strings.Builder
	n, size := p.count()
	fmt.Fprintf(&b, "%s\n\n", emphStyle.Render(fmt.Sprintf("Zap %d worktree%s (about %s)?", n, plural(n), sizer.Human(size))))
	shown := 0
	for _, r := range p.rows {
		if r.header || !p.selected[Key{r.plan, r.item}] {
			continue
		}
		if shown == p.bodyHeight()-4 {
			fmt.Fprintf(&b, "  … and %d more\n", n-shown)
			break
		}
		it := p.item(r)
		fmt.Fprintf(&b, "  %s  %s\n", p.short(it.Path), helpStyle.Render(it.Describe()))
		shown++
	}
	b.WriteString("\nBranches are backed up under refs/zap/backup/ first; `zap undo` brings them back.\n")
	b.WriteString(warnStyle.Render("Ignored files (node_modules, build output, .env) are deleted for good.") + "\n\n")
	b.WriteString(emphStyle.Render("y") + " zap · " + emphStyle.Render("n") + " back\n")
	return b.String()
}

func (p *Picker) short(path string) string {
	if p.home != "" {
		if rest, ok := strings.CutPrefix(path, p.home+string(filepath.Separator)); ok {
			return "~/" + rest
		}
	}
	return path
}

func (p *Picker) rel(path, repo string) string {
	if r, err := filepath.Rel(repo, path); err == nil && !strings.HasPrefix(r, "..") {
		return clip(r, 34)
	}
	return clip(p.short(path), 34)
}

func clip(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return "…" + string([]rune(s)[len([]rune(s))-n+1:])
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
