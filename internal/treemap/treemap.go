// Package treemap lays weighted items out as nested rectangles on a character grid,
// with the squarified algorithm (Bruls, Huizing and van Wijk, 2000): rows of blocks
// are added while they keep the worst aspect ratio from getting worse, so blocks stay
// close to square and their labels fit. Terminal cells are about twice as tall as
// they are wide, which the layout corrects for.
package treemap

import (
	"math"
	"sort"
)

// Rect is a block on the grid, in cells.
type Rect struct{ X, Y, W, H int }

// Node is one thing to draw. A node with children is laid out inside its own block.
type Node struct {
	Label    string
	Weight   int64
	Kind     int // the caller's category, for colour and fill
	Children []*Node
	Rect     Rect // set by Layout
}

// cellAspect is a terminal cell's height over its width.
const cellAspect = 2.0

// Layout places nodes in r, largest first, and recurses into children, leaving a
// one-cell header inside every parent block for its label. Nodes with no weight are
// left at the zero Rect.
func Layout(nodes []*Node, r Rect) {
	items := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Weight > 0 {
			items = append(items, n)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Weight > items[j].Weight })
	place(items, r)
	for _, n := range items {
		if len(n.Children) == 0 || n.Rect.W < 4 || n.Rect.H < 3 {
			continue
		}
		in := Rect{n.Rect.X + 1, n.Rect.Y + 1, n.Rect.W - 2, n.Rect.H - 2}
		Layout(n.Children, in)
	}
}

// place lays items (sorted, all with weight) into r with squarified rows.
func place(items []*Node, r Rect) {
	if len(items) == 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	var total float64
	for _, n := range items {
		total += float64(n.Weight)
	}
	// Work in square units: height counts cellAspect times.
	free := frect{float64(r.X), float64(r.Y) * cellAspect, float64(r.W), float64(r.H) * cellAspect}
	scale := free.w * free.h / total
	fr := map[*Node]frect{}
	var row []*Node
	for i := 0; i < len(items); {
		n := items[i]
		side := math.Min(free.w, free.h)
		if len(row) == 0 || worst(append(row, n), side, scale) <= worst(row, side, scale) {
			row = append(row, n)
			i++
			continue
		}
		free = layRow(fr, row, free, scale)
		row = nil
	}
	if len(row) > 0 {
		layRow(fr, row, free, scale)
	}
	snap(fr, items, r)
}

type frect struct{ x, y, w, h float64 }

// layRow records each node's exact position in fr and returns the space left.
func layRow(fr map[*Node]frect, row []*Node, free frect, scale float64) frect {
	var sum float64
	for _, n := range row {
		sum += float64(n.Weight) * scale
	}
	if free.w >= free.h { // a column on the left
		w := sum / free.h
		y := free.y
		for _, n := range row {
			h := float64(n.Weight) * scale / w
			fr[n] = frect{free.x, y, w, h}
			y += h
		}
		return frect{free.x + w, free.y, free.w - w, free.h}
	}
	h := sum / free.w // a row along the top
	x := free.x
	for _, n := range row {
		w := float64(n.Weight) * scale / h
		fr[n] = frect{x, free.y, w, h}
		x += w
	}
	return frect{free.x, free.y + h, free.w, free.h - h}
}

func worst(row []*Node, side, scale float64) float64 {
	var sum, lo, hi float64
	lo = math.Inf(1)
	for _, n := range row {
		a := float64(n.Weight) * scale
		sum += a
		lo, hi = math.Min(lo, a), math.Max(hi, a)
	}
	s2, w2 := sum*sum, side*side
	return math.Max(w2*hi/s2, s2/(w2*lo))
}

// snap rounds exact positions to cells so neighbours share edges and the blocks
// tile r exactly.
func snap(fr map[*Node]frect, items []*Node, r Rect) {
	for _, n := range items {
		f := fr[n]
		x0, x1 := int(math.Round(f.x)), int(math.Round(f.x+f.w))
		y0, y1 := int(math.Round(f.y/cellAspect)), int(math.Round((f.y+f.h)/cellAspect))
		x1, y1 = min(x1, r.X+r.W), min(y1, r.Y+r.H)
		n.Rect = Rect{x0, y0, max(0, x1-x0), max(0, y1-y0)}
	}
}
