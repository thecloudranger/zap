package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/agent"
	"github.com/thecloudranger/zap/internal/policy"
)

// defaultRoots are searched when the config sets none.
func defaultRoots() []string {
	roots := []string{"~/dev", "~/code", "~/src", "~/projects"}
	for _, f := range agent.Folders() {
		roots = append(roots, "~/"+f)
	}
	return roots
}

// configuredRoots returns the roots as written (with ~) and whether they are defaults.
func configuredRoots(d Deps) ([]string, bool, error) {
	cfg, err := policy.LoadConfig(d.ConfigDir)
	if err != nil {
		return nil, false, err
	}
	if len(cfg.Scan.Roots) == 0 {
		return defaultRoots(), true, nil
	}
	return cfg.Scan.Roots, false, nil
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}

// RootsResult is the `data` payload of `zap roots --json`.
type RootsResult struct {
	Roots    []RootInfo `json:"roots"`
	Defaults bool       `json:"defaults"`
}

// RootInfo is one scan root.
type RootInfo struct {
	Root   string `json:"root"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

func newRootsCmd(d Deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "roots",
		Short: "Show the folders `zap scan --all` searches for repositories",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			roots, defaults, err := configuredRoots(d)
			if err != nil {
				return err
			}
			res := RootsResult{Defaults: defaults}
			for _, r := range roots {
				p := expandHome(r, d.Home)
				_, statErr := os.Stat(p)
				res.Roots = append(res.Roots, RootInfo{Root: r, Path: p, Exists: statErr == nil})
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return writeOK(out, "roots", res)
			}
			for _, r := range res.Roots {
				note := ""
				if !r.Exists {
					note = "  (does not exist, skipped)"
				}
				_, _ = fmt.Fprintf(out, "  %s%s\n", r.Root, note)
			}
			if defaults {
				_, _ = fmt.Fprintln(out, "\nThese are the defaults. Change them with `zap roots add PATH` / `zap roots remove PATH`.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output a JSON envelope on stdout")
	cmd.AddCommand(newRootsEditCmd(d, "add"), newRootsEditCmd(d, "remove"), newRootsResetCmd(d))
	return cmd
}

func newRootsEditCmd(d Deps, verb string) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " PATH...",
		Short: strings.ToUpper(verb[:1]) + verb[1:] + " scan roots",
		Args:  usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := policy.LoadConfig(d.ConfigDir)
			if err != nil {
				return err
			}
			roots := cfg.Scan.Roots
			if len(roots) == 0 {
				roots = defaultRoots() // the first edit starts from the defaults
			}
			for _, a := range args {
				r, err := normalizeRoot(a, d.Home)
				if err != nil {
					return err
				}
				i := slices.Index(roots, r)
				switch {
				case verb == "add" && i < 0:
					roots = append(roots, r)
				case verb == "remove" && i >= 0:
					roots = slices.Delete(roots, i, i+1)
				case verb == "remove":
					return usageErr(fmt.Errorf("%s is not a scan root (see `zap roots`)", r))
				}
			}
			if len(roots) == 0 {
				return usageErr(fmt.Errorf("at least one root is required; use `zap roots reset` for the defaults"))
			}
			cfg.Scan.Roots = roots
			if err := policy.SaveConfig(d.ConfigDir, cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Scan roots: %s\n", strings.Join(roots, ", "))
			return nil
		},
	}
}

func newRootsResetCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Go back to the default scan roots",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := policy.LoadConfig(d.ConfigDir)
			if err != nil {
				return err
			}
			cfg.Scan.Roots = nil
			if err := policy.SaveConfig(d.ConfigDir, cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Scan roots reset to the defaults.")
			return nil
		},
	}
}

// normalizeRoot makes a path absolute and writes it with ~ when under home.
func normalizeRoot(p, home string) (string, error) {
	abs, err := filepath.Abs(expandHome(p, home))
	if err != nil {
		return "", err
	}
	if home != "" {
		if rest, ok := strings.CutPrefix(abs, home+string(filepath.Separator)); ok {
			return "~/" + filepath.ToSlash(rest), nil
		}
	}
	return abs, nil
}
