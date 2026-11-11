package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/model"
)

// ScanResult is the `data` payload of `zap scan --json`.
type ScanResult struct {
	Repos []model.RepoReport `json:"repos"`
}

func newScanCmd(d Deps) *cobra.Command {
	var (
		asJSON, fetch bool
		target        string
	)
	cmd := &cobra.Command{
		Use:   "scan [PATH]",
		Short: "List worktrees of the repository at PATH (default: current directory) with their status",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			res, err := scan(cmd, d, args, target, fetch)
			if asJSON {
				if err != nil {
					return writeErr(out, "scan", err)
				}
				return writeJSON(out, Envelope{Command: "scan", Status: "ok", Data: res, Warnings: res.warnings()})
			}
			if err != nil {
				return err
			}
			return renderScan(out, res, d.Home, d.Now())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	cmd.Flags().StringVar(&target, "target", "", "branch that counts as merged-into (default: origin/HEAD, main, master)")
	cmd.Flags().BoolVar(&fetch, "fetch", false, "fetch all remotes first so merged pull requests are seen")
	return cmd
}

func (r ScanResult) warnings() []string {
	var w []string
	for _, rep := range r.Repos {
		w = append(w, rep.Warnings...)
	}
	return w
}

func scan(cmd *cobra.Command, d Deps, args []string, target string, fetch bool) (ScanResult, error) {
	dir, err := targetDir(d, args)
	if err != nil {
		return ScanResult{}, err
	}
	rep, err := inspect.Repo(cmd.Context(), d.Git, dir, inspect.Options{Target: target, Fetch: fetch})
	if err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Repos: []model.RepoReport{rep}}, nil
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
