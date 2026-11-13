package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/thecloudranger/zap/internal/schedule"
)

func newScheduleCmd(d Deps) *cobra.Command {
	var every time.Duration
	cmd := &cobra.Command{
		Use:   "schedule [on|off|status]",
		Short: "Run `zap check --notify` on a timer (launchd on macOS, systemd on Linux)",
		Long: "Install, remove or show a periodic `zap check --notify`. It only reports and notifies;\n" +
			"it never deletes anything. On macOS it is a launchd agent, on Linux a systemd user timer.\n" +
			"On Termux, which has neither, it prints a cron line to add yourself.",
		Args:      usageArgs(cobra.MaximumNArgs(1)),
		ValidArgs: []string{"on", "off", "status"},
		RunE: func(cmd *cobra.Command, args []string) error {
			action := "status"
			if len(args) == 1 {
				action = args[0]
			}
			if every < time.Hour {
				return usageErr(errors.New("--every must be at least 1h"))
			}
			e, err := scheduleEnv(d)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if e.Termux || (e.GOOS != "darwin" && e.GOOS != "linux") {
				_, _ = fmt.Fprintf(out, "zap cannot install a timer here. Add this line with `crontab -e`:\n\n  %s\n", e.CronLine())
				return nil
			}
			switch action {
			case "on":
				paths, err := e.On(cmd.Context(), every)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(out, "zap check --notify now runs every %s. It never deletes anything.\n", human(every))
				for _, p := range paths {
					_, _ = fmt.Fprintf(out, "  wrote %s\n", tildify(p, d.Home))
				}
				_, _ = fmt.Fprintf(out, "Output goes to %s. Turn it off with zap schedule off.\n", tildify(filepath.Join(e.StateDir, "check.log"), d.Home))
			case "off":
				removed, err := e.Off(cmd.Context())
				if err != nil {
					return err
				}
				if len(removed) == 0 {
					_, _ = fmt.Fprintln(out, "No schedule was installed.")
					return nil
				}
				for _, p := range removed {
					_, _ = fmt.Fprintf(out, "  removed %s\n", tildify(p, d.Home))
				}
				_, _ = fmt.Fprintln(out, "The schedule is off.")
			case "status":
				st, err := e.Status()
				if err != nil {
					return err
				}
				if !st.On {
					_, _ = fmt.Fprintln(out, "No schedule. Turn one on with zap schedule on.")
					return nil
				}
				_, _ = fmt.Fprintln(out, "zap check --notify is scheduled:")
				for _, p := range st.Files {
					_, _ = fmt.Fprintf(out, "  %s\n", tildify(p, d.Home))
				}
			default:
				return usageErr(fmt.Errorf("unknown action %q: use on, off or status", action))
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&every, "every", 24*time.Hour, "how often to check (at least 1h)")
	return cmd
}

func scheduleEnv(d Deps) (schedule.Env, error) {
	exe, err := zapPath()
	if err != nil {
		return schedule.Env{}, err
	}
	run := d.Exec
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}
	return schedule.Env{GOOS: runtime.GOOS, Termux: os.Getenv("TERMUX_VERSION") != "", Home: d.Home,
		UID: os.Getuid(), Exe: exe, StateDir: d.StateDir, Run: run}, nil
}

// zapPath is the zap a timer should run: the PATH entry when it is this binary, so a
// Homebrew upgrade (which moves the real file) does not break the schedule.
func zapPath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cli: locate zap: %w", err)
	}
	if onPath, err := exec.LookPath("zap"); err == nil {
		a, errA := filepath.EvalSymlinks(onPath)
		b, errB := filepath.EvalSymlinks(self)
		if errA == nil && errB == nil && a == b {
			if abs, err := filepath.Abs(onPath); err == nil {
				return abs, nil
			}
		}
	}
	return self, nil
}

func human(dur time.Duration) string {
	if dur%(24*time.Hour) == 0 {
		if n := int(dur / (24 * time.Hour)); n > 1 {
			return fmt.Sprintf("%d days", n)
		}
		return "day"
	}
	if dur%time.Hour == 0 {
		return fmt.Sprintf("%d hours", int(dur/time.Hour))
	}
	return dur.String()
}
