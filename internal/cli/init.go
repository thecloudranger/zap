package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/policy"
)

// ErrAlreadyOptedIn is returned by init when .zap exists.
var ErrAlreadyOptedIn = errors.New("already opted in")

func newInitCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init [PATH]",
		Short: "Opt a repository in to cleaning by writing a .zap file at its root",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := targetDir(d, args)
			if err != nil {
				return err
			}
			repo, err := gitx.ListRepo(cmd.Context(), d.Git, dir)
			if err != nil {
				return err
			}
			path := filepath.Join(repo.Root, policy.FileName)
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("cli: %s exists: %w", tildify(path, d.Home), ErrAlreadyOptedIn)
			}
			if err != nil {
				return fmt.Errorf("cli: %w", err)
			}
			if _, err := f.WriteString(policy.Template); err != nil {
				_ = f.Close()
				return fmt.Errorf("cli: write %s: %w", path, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("cli: %w", err)
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Wrote %s\n\n", tildify(path, d.Home))
			_, _ = fmt.Fprintln(out, "zap may now remove worktrees of this repository that pass every safety check.")
			_, _ = fmt.Fprintln(out, "Commit .zap to share these rules with everyone who works on it.")
			_, _ = fmt.Fprintln(out, "Preview what would happen with: zap clean --dry-run")
			return nil
		},
	}
}
