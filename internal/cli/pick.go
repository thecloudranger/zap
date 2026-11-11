package cli

import (
	"bufio"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/plan"
	"github.com/thecloudranger/zap/internal/sizer"
	"github.com/thecloudranger/zap/internal/tui"
)

// runPicker is `zap` with no subcommand on a terminal: plan, let the user choose in the
// picker, then confirm the disclaimer and apply exactly what was chosen.
func runPicker(cmd *cobra.Command, d Deps, all bool) error {
	out := cmd.OutOrStdout()
	cwd, _ := d.Getwd()
	fl := cleanFlags{all: all}
	plans, warnings, err := buildPlans(cmd, d, nil, fl, cwd)
	if !all && errors.Is(err, gitx.ErrNotRepo) {
		fl.all = true // outside a repository: every repository under the scan roots
		plans, warnings, err = buildPlans(cmd, d, nil, fl, cwd)
	}
	if err != nil {
		return err
	}
	for _, w := range warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}
	if !hasItems(plans) {
		_, _ = fmt.Fprintln(out, "Nothing to zap: no merged, empty or missing worktrees.")
		return nil
	}

	optIn := func(repo string) (plan.Plan, error) {
		if _, err := writeZap(repo); err != nil {
			return plan.Plan{}, err
		}
		return plan.Build(cmd.Context(), d.Git, repo, plan.Options{Now: d.Now(), Cwd: cwd, Sizer: sizer.NewCounter()})
	}
	picker := tui.New(plans, d.Home, optIn)
	prog := tea.NewProgram(picker, tea.WithContext(cmd.Context()), tea.WithInput(d.In), tea.WithOutput(d.Out))
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("cli: picker: %w", err)
	}
	if !picker.Confirmed() {
		_, _ = fmt.Fprintln(out, "Nothing was changed.")
		return nil
	}

	plans = keepUnselected(picker.Plans(), picker.Selected())
	if err := ensureDisclaimer(d, bufio.NewReader(d.In), out, fl); err != nil {
		return err
	}
	return applyAndReport(cmd, d, plans, nil, fl, cwd)
}

func hasItems(plans []plan.Plan) bool {
	for _, p := range plans {
		if len(p.Items) > 0 {
			return true
		}
	}
	return false
}

// keepUnselected makes Apply touch exactly the selection: unselected items become Keep,
// and a selected optional (idle) item becomes a removal. Nothing else can change: an item
// a safety gate refused is never selectable.
func keepUnselected(plans []plan.Plan, sel map[tui.Key]bool) []plan.Plan {
	out := make([]plan.Plan, len(plans))
	for pi, p := range plans {
		p.Items = append([]plan.Item(nil), p.Items...)
		for ii := range p.Items {
			it := &p.Items[ii]
			chosen := sel[tui.Key{Plan: pi, Item: ii}] && tui.Selectable(p, *it)
			switch {
			case !chosen && it.Action != plan.Keep:
				it.Action, it.Refusal = plan.Keep, "not selected"
			case chosen && it.Action == plan.Keep: // an idle item picked by hand
				it.Action, it.Refusal = plan.Remove, ""
			}
		}
		out[pi] = p
	}
	return out
}
