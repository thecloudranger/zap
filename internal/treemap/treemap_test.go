package treemap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func leaves(ns []*Node) []*Node {
	var out []*Node
	for _, n := range ns {
		if len(n.Children) == 0 {
			out = append(out, n)
		} else {
			out = append(out, leaves(n.Children)...)
		}
	}
	return out
}

func TestLayoutTilesExactly(t *testing.T) {
	for _, weights := range [][]int64{{1}, {5, 5}, {100, 50, 25, 12, 6, 3, 1}, {1e9, 1, 1}, {7, 7, 7, 7, 7, 7, 7, 7, 7}} {
		var ns []*Node
		for _, w := range weights {
			ns = append(ns, &Node{Weight: w})
		}
		r := Rect{2, 1, 96, 24}
		Layout(ns, r)
		grid := map[[2]int]int{}
		for i, n := range ns {
			assert.GreaterOrEqual(t, n.Rect.X, r.X)
			assert.LessOrEqual(t, n.Rect.X+n.Rect.W, r.X+r.W, "%v", weights)
			assert.LessOrEqual(t, n.Rect.Y+n.Rect.H, r.Y+r.H, "%v", weights)
			for x := n.Rect.X; x < n.Rect.X+n.Rect.W; x++ {
				for y := n.Rect.Y; y < n.Rect.Y+n.Rect.H; y++ {
					prev, taken := grid[[2]int{x, y}]
					assert.False(t, taken, "%v: blocks %d and %d overlap at %d,%d", weights, prev, i, x, y)
					grid[[2]int{x, y}] = i
				}
			}
		}
		assert.Len(t, grid, r.W*r.H, "%v: the blocks cover the area with no gaps", weights)
	}
}

func TestLayoutProportionsAndNesting(t *testing.T) {
	big, small := &Node{Weight: 300}, &Node{Weight: 100}
	parent := &Node{Weight: 400, Children: []*Node{big, small}}
	other := &Node{Weight: 400}
	Layout([]*Node{parent, other}, Rect{0, 0, 80, 20})
	assert.InDelta(t, other.Rect.W*other.Rect.H, parent.Rect.W*parent.Rect.H, 40, "equal weights, about equal areas")
	in := Rect{parent.Rect.X + 1, parent.Rect.Y + 1, parent.Rect.W - 2, parent.Rect.H - 2}
	for _, c := range []*Node{big, small} {
		assert.GreaterOrEqual(t, c.Rect.X, in.X)
		assert.GreaterOrEqual(t, c.Rect.Y, in.Y)
		assert.LessOrEqual(t, c.Rect.X+c.Rect.W, in.X+in.W)
		assert.LessOrEqual(t, c.Rect.Y+c.Rect.H, in.Y+in.H)
	}
	assert.Greater(t, big.Rect.W*big.Rect.H, 2*small.Rect.W*small.Rect.H)
	assert.Len(t, leaves([]*Node{parent, other}), 3)

	zero := &Node{Weight: 0}
	Layout([]*Node{zero, {Weight: 1}}, Rect{0, 0, 10, 5})
	assert.Equal(t, Rect{}, zero.Rect, "nothing to draw for an empty node")
}
