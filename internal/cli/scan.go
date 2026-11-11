package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
)

// ScanResult is the `data` payload of `zap scan --json`.
type ScanResult struct {
	Repos []model.Repo `json:"repos"`
}

func newScanCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "scan [PATH]",
		Short: "List worktrees of the repository at PATH (default: current directory)",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			res, err := scan(cmd, d, args)
			if asJSON {
				if err != nil {
					return writeErr(out, "scan", err)
				}
				return writeOK(out, "scan", res)
			}
			if err != nil {
				return err
			}
			return renderScan(out, res, d.Home)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	return cmd
}

func scan(cmd *cobra.Command, d Deps, args []string) (ScanResult, error) {
	dir, err := targetDir(d, args)
	if err != nil {
		return ScanResult{}, err
	}
	repo, err := gitx.ListRepo(cmd.Context(), d.Git, dir)
	if err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Repos: []model.Repo{repo}}, nil
}

func targetDir(d Deps, args []string) (string, error) {
	if len(args) == 1 {
		return filepath.Abs(args[0])
	}
	wd, err := d.Getwd()
	if err != nil {
		return "", fmt.Errorf("cli: working directory: %w", err)
	}
	return wd, nil
}

func renderScan(w io.Writer, res ScanResult, home string) error {
	for _, repo := range res.Repos {
		linked := repo.Linked()
		if _, err := fmt.Fprintf(w, "%s  ·  %d linked worktree%s\n\n", tildify(repo.Root, home), len(linked), plural(len(linked))); err != nil {
			return err
		}
		if len(linked) == 0 {
			continue
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  WORKTREE\tBRANCH\tHEAD\tSTATE")
		for _, wt := range linked {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", relTo(wt.Path, repo.Root, home), branchLabel(wt), short(wt.Head), stateLabel(wt))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func branchLabel(wt model.Worktree) string {
	switch {
	case wt.Branch != "":
		return wt.Branch
	case wt.Detached:
		return "(detached)"
	default:
		return "-"
	}
}

func stateLabel(wt model.Worktree) string {
	var s []string
	if wt.Locked {
		s = append(s, "locked")
	}
	if wt.Prunable {
		s = append(s, "missing")
	}
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ",")
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
