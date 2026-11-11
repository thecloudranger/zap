package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thecloudranger/zap/internal/model"
)

func renderScan(w io.Writer, res ScanResult, home string, now time.Time) error {
	for _, warn := range res.Warnings {
		_, _ = fmt.Fprintf(w, "warning: %s\n", warn)
	}
	if len(res.Warnings) > 0 {
		_, _ = fmt.Fprintln(w)
	}
	shown := 0
	for _, rep := range res.Repos {
		linked := linkedOf(rep)
		if res.All && len(linked) == 0 {
			continue // machine-wide: only repositories that have worktrees
		}
		shown++
		for _, warn := range rep.Warnings {
			_, _ = fmt.Fprintf(w, "warning: %s\n", warn)
		}
		_, _ = fmt.Fprintf(w, "%s  ·  %d linked worktree%s  ·  target %s\n\n",
			tildify(rep.Root, home), len(linked), plural(len(linked)), rep.Target.Name)
		if len(linked) == 0 {
			continue
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  WORKTREE\tBRANCH\tAGENT\tSTATUS\tAGE\tACTIVE\tNOTES")
		for _, a := range linked {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				relTo(a.Path, rep.Root, home), branchLabel(a.Worktree), dash(a.Agent), a.Verdict.Status,
				ago(now, a.Facts.Created), ago(now, a.Facts.LastActivity), notes(a))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w, "\n"+summaryLine(linked)+"\n")
	}
	if res.All {
		_, _ = fmt.Fprintf(w, "Scanned %d repositor%s in %s; %d with linked worktrees. Roots: zap roots\n",
			res.Scanned, pluralY(res.Scanned), res.Elapsed.Round(100*time.Millisecond), shown)
	}
	return nil
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func linkedOf(rep model.RepoReport) []model.Assessment {
	out := make([]model.Assessment, 0, len(rep.Worktrees))
	for _, a := range rep.Worktrees {
		if !a.Main {
			out = append(out, a)
		}
	}
	return out
}

func summaryLine(as []model.Assessment) string {
	counts := map[model.Status]int{}
	for _, a := range as {
		counts[a.Verdict.Status]++
	}
	var parts []string
	for _, s := range []model.Status{model.StatusMerged, model.StatusEmpty, model.StatusGone, model.StatusMissing, model.StatusActive} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return strings.Join(parts, " · ")
}

func notes(a model.Assessment) string {
	var s []string
	if a.Locked {
		s = append(s, "locked")
	}
	if a.Facts.Dirty {
		s = append(s, fmt.Sprintf("dirty(%d)", a.Facts.Changes))
	}
	if a.Detached {
		s = append(s, "detached")
	}
	if len(a.Errors) > 0 {
		s = append(s, "errors")
	}
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ",")
}

func branchLabel(wt model.Worktree) string {
	switch {
	case wt.Branch != "":
		return wt.Branch
	case wt.Detached:
		return short(wt.Head)
	default:
		return "-"
	}
}

// ago renders a coarse, human age; zero times render as "-".
func ago(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 10*7*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
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

// relTo shows worktrees inside the repo relative to it, others with ~ for home.
func relTo(p, root, home string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return tildify(p, home)
}

func tildify(p, home string) string {
	if home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}
	return p
}
