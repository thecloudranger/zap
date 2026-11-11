package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/thecloudranger/zap/internal/discover"
	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/inspect"
	"github.com/thecloudranger/zap/internal/model"
)

// ScanResult is the `data` payload of `zap scan --json`.
type ScanResult struct {
	All      bool               `json:"all"` // machine-wide scan of the scan roots
	Roots    []string           `json:"roots,omitempty"`
	Repos    []model.RepoReport `json:"repos"`
	Scanned  int                `json:"scanned"` // repositories inspected
	Warnings []string           `json:"-"`
	Elapsed  time.Duration      `json:"-"`
}

type scanFlags struct {
	asJSON, fetch, all bool
	target             string
}

func newScanCmd(d Deps) *cobra.Command {
	var fl scanFlags
	cmd := &cobra.Command{
		Use:   "scan [PATH]",
		Short: "List worktrees and their status: of the current repo, of PATH, or of every repo (--all)",
		Long: "List worktrees and their status.\n\n" +
			"Inside a repository, scans that repository. Outside one, or with --all, scans every\n" +
			"repository under the scan roots (see `zap roots`).",
		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fl.all && len(args) > 0 {
				return usageErr(errors.New("--all and PATH cannot be used together"))
			}
			out := cmd.OutOrStdout()
			res, err := scan(cmd, d, args, fl)
			if fl.asJSON {
				if err != nil {
					return writeErr(out, "scan", err)
				}
				return writeJSON(out, Envelope{Command: "scan", Status: "ok", Data: res, Warnings: res.allWarnings()})
			}
			if err != nil {
				return err
			}
			return renderScan(out, res, d.Home, d.Now())
		},
	}
	f := cmd.Flags()
	f.BoolVar(&fl.asJSON, "json", false, "output a JSON envelope on stdout")
	f.StringVar(&fl.target, "target", "", "branch that counts as merged-into (default: origin/HEAD, main, master)")
	f.BoolVar(&fl.fetch, "fetch", false, "fetch all remotes first so merged pull requests are seen")
	f.BoolVarP(&fl.all, "all", "a", false, "scan every repository under the scan roots")
	return cmd
}

func (r ScanResult) allWarnings() []string {
	w := append([]string(nil), r.Warnings...)
	for _, rep := range r.Repos {
		for _, x := range rep.Warnings {
			w = append(w, rep.Root+": "+x)
		}
	}
	return w
}

func scan(cmd *cobra.Command, d Deps, args []string, fl scanFlags) (ScanResult, error) {
	opt := inspect.Options{Target: fl.target, Fetch: fl.fetch}
	if !fl.all {
		dir, err := targetDir(d, args)
		if err != nil {
			return ScanResult{}, err
		}
		rep, err := inspect.Repo(cmd.Context(), d.Git, dir, opt)
		switch {
		case err == nil:
			return ScanResult{Repos: []model.RepoReport{rep}, Scanned: 1}, nil
		case len(args) > 0 || !errors.Is(err, gitx.ErrNotRepo):
			return ScanResult{}, err
		}
		// Not inside a repository and no PATH: fall through to a machine-wide scan.
	}
	return scanAll(cmd, d, opt, fl.asJSON)
}

// scanAll discovers every repository under the scan roots and inspects them concurrently.
// A repository that cannot be inspected becomes a warning, not a failure.
func scanAll(cmd *cobra.Command, d Deps, opt inspect.Options, quiet bool) (ScanResult, error) {
	start := time.Now()
	roots, _, err := configuredRoots(d)
	if err != nil {
		return ScanResult{}, err
	}
	abs := make([]string, len(roots))
	for i, r := range roots {
		abs[i] = expandHome(r, d.Home)
	}
	progress := newProgress(cmd, d, quiet)
	found, err := discover.Find(cmd.Context(), abs, discover.Options{Progress: progress.folders})
	if err != nil {
		progress.done()
		return ScanResult{}, err
	}
	res := ScanResult{All: true, Roots: roots, Warnings: found.Warnings}

	reports := make([]*model.RepoReport, len(found.Repos))
	errs := make([]error, len(found.Repos))
	g, ctx := errgroup.WithContext(cmd.Context())
	g.SetLimit(4) // each repository is itself inspected concurrently
	for i, dir := range found.Repos {
		g.Go(func() error {
			rep, err := inspect.Repo(ctx, d.Git, dir, opt)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				errs[i] = err
			} else {
				reports[i] = &rep
			}
			progress.repo(len(found.Repos))
			return nil
		})
	}
	err = g.Wait()
	progress.done()
	if err != nil {
		return ScanResult{}, err
	}
	for i, rep := range reports {
		if rep != nil {
			res.Repos = append(res.Repos, *rep)
			res.Scanned++
		} else {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", tildify(found.Repos[i], d.Home), errs[i]))
		}
	}
	res.Elapsed = time.Since(start)
	return res, nil
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

// progress prints a single updating status line on stderr while scanning on a terminal.
type progress struct {
	mu      sync.Mutex
	cmd     *cobra.Command
	on      bool
	repos   int
	printed bool
}

func newProgress(cmd *cobra.Command, d Deps, quiet bool) *progress {
	return &progress{cmd: cmd, on: !quiet && d.IsTTY()}
}

func (p *progress) folders(n int) {
	p.print(fmt.Sprintf("searching… %d folders", n))
}

func (p *progress) repo(total int) {
	p.mu.Lock()
	p.repos++
	n := p.repos
	p.mu.Unlock()
	p.print(fmt.Sprintf("inspecting… %d/%d repositories", n, total))
}

func (p *progress) print(s string) {
	if !p.on {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(p.cmd.ErrOrStderr(), "\r\033[K%s", s)
	p.printed = true
}

func (p *progress) done() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.printed {
		_, _ = fmt.Fprint(p.cmd.ErrOrStderr(), "\r\033[K")
	}
}
