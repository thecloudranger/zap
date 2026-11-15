package plan

import (
	"fmt"
	"sort"
	"strings"
)

// Reasons a worktree is kept that are not safety gates. The others are gate names.
const (
	KeptUnmerged   = "unmerged"     // its status is not in the repo's rules (usually active)
	KeptNotOptedIn = "not-opted-in" // the repo has no .zap file
	KeptIdle       = "idle"         // only removed when picked in the picker
	KeptUnselected = "unselected"   // the picker offered it and the person left it out
)

// keptLabels says why a worktree is kept, for one and for several. Keys are gate names
// (see safety) and the Kept* constants; order is the tie-break when counts are equal.
var keptLabels = []struct{ key, one, many string }{
	{KeptUnmerged, "has unmerged work", "have unmerged work"},
	{"clean", "has uncommitted changes", "have uncommitted changes"},
	{"old-enough", "was used recently", "were used recently"},
	{"not-in-use", "has a shell or agent running in it", "have a shell or agent running in them"},
	{"keep-ignored", "holds an ignored file to keep, like .env", "hold ignored files to keep, like .env"},
	{"no-hidden", "has changes hidden from git status", "have changes hidden from git status"},
	{"unlocked", "is locked", "are locked"},
	{"unprotected", "is on a protected branch", "are on a protected branch"},
	{"not-cwd", "is the folder you ran zap from", "contain the folder you ran zap from"},
	{"inspected", "couldn't be checked", "couldn't be checked"},
	{KeptIdle, "is idle (pick it in zap to remove it)", "are idle (pick them in zap to remove them)"},
	{KeptNotOptedIn, "is in a repo without a .zap file", "are in repos without a .zap file"},
	{KeptUnselected, "wasn't selected", "weren't selected"},
}

// KeptCount is how many worktrees one reason kept.
type KeptCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// Kept counts kept worktrees by reason, most common first. With picked nil an item is kept
// when its action is Keep (clean); the picker passes its selection instead, so an item is
// kept when it is not picked, and a removal the person left out counts as KeptUnselected.
func Kept(plans []Plan, picked func(plan, item int) bool) []KeptCount {
	counts := map[string]int{}
	for pi, p := range plans {
		counts[KeptUnmerged] += p.ActiveCount
		for ii, it := range p.Items {
			var reason string
			switch {
			case picked != nil && picked(pi, ii):
				continue
			case it.Action != Keep && picked == nil:
				continue
			case it.Action != Keep:
				reason = KeptUnselected
			case it.KeptBy == "":
				reason = "inspected"
			default:
				reason = it.KeptBy
			}
			counts[reason]++
		}
	}
	order := map[string]int{}
	for i, l := range keptLabels {
		order[l.key] = i
	}
	var out []KeptCount
	for r, n := range counts {
		if n > 0 {
			out = append(out, KeptCount{Reason: r, Count: n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		oi, iok := order[out[i].Reason]
		oj, jok := order[out[j].Reason]
		if iok != jok {
			return iok
		}
		if oi != oj {
			return oi < oj
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// KeptLine is one sentence saying why the rest stay, or "" when nothing was kept.
// Example: "Why the other 5 stay: 3 have unmerged work, 2 have uncommitted changes."
func KeptLine(kc []KeptCount) string {
	total := 0
	parts := make([]string, 0, len(kc))
	for _, k := range kc {
		total += k.Count
		parts = append(parts, fmt.Sprintf("%d %s", k.Count, keptLabel(k.Reason, k.Count)))
	}
	if total == 0 {
		return ""
	}
	if total == 1 {
		return "Why the other one stays: it " + strings.TrimPrefix(parts[0], "1 ") + "."
	}
	return fmt.Sprintf("Why the other %d stay: %s.", total, strings.Join(parts, ", "))
}

func keptLabel(reason string, n int) string {
	for _, l := range keptLabels {
		if l.key == reason {
			if n == 1 {
				return l.one
			}
			return l.many
		}
	}
	if n == 1 {
		return "was kept by the " + reason + " check"
	}
	return "were kept by the " + reason + " check"
}
