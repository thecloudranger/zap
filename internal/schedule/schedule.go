// Package schedule installs a periodic `zap check --notify`: a launchd agent on macOS,
// a systemd user timer on Linux. On Termux, which has neither, it only describes a
// cron line. The scheduled job never deletes anything; it reports and notifies.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Label names the launchd agent and the systemd units.
const (
	Label      = "dev.zap.check"
	unitPrefix = "zap-check"
)

// ErrUnsupported is returned where zap cannot install a schedule itself.
var ErrUnsupported = errors.New("schedule: no launchd or systemd here")

// Runner runs a command and returns its combined output; tests swap it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Env describes the machine.
type Env struct {
	GOOS     string
	Termux   bool
	Home     string
	UID      int
	Exe      string // the zap to run, as found on PATH
	StateDir string // where the job's output goes
	Run      Runner
}

// Status says whether a schedule is installed and where.
type Status struct {
	On    bool     `json:"on"`
	Files []string `json:"files"`
}

// Files returns the unit files On writes, path to content.
func (e Env) Files(every time.Duration) (map[string]string, error) {
	switch {
	case e.Termux:
		return nil, ErrUnsupported
	case e.GOOS == "darwin":
		return map[string]string{e.plistPath(): e.plist(every)}, nil
	case e.GOOS == "linux":
		dir := e.systemdDir()
		return map[string]string{
			filepath.Join(dir, unitPrefix+".service"): e.service(),
			filepath.Join(dir, unitPrefix+".timer"):   timer(every),
		}, nil
	}
	return nil, ErrUnsupported
}

// CronLine is the crontab entry for machines zap cannot schedule on itself.
func (e Env) CronLine() string {
	return "0 10 * * * " + shellQuote(e.Exe) + " check --notify"
}

// On writes the unit files and loads them, replacing an earlier schedule.
func (e Env) On(ctx context.Context, every time.Duration) ([]string, error) {
	files, err := e.Files(every)
	if err != nil {
		return nil, err
	}
	_ = e.unload(ctx) // a schedule that is already loaded is replaced
	var paths []string
	for p, content := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := os.MkdirAll(e.StateDir, 0o755); err != nil {
		return nil, err
	}
	return paths, e.load(ctx)
}

// Off unloads the schedule and removes its files. It is not an error when none exists.
func (e Env) Off(ctx context.Context) ([]string, error) {
	files, err := e.Files(time.Hour)
	if err != nil {
		return nil, err
	}
	_ = e.unload(ctx)
	var removed []string
	for p := range files {
		if err := os.Remove(p); err == nil {
			removed = append(removed, p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
	}
	if e.GOOS == "linux" {
		_, _ = e.Run(ctx, "systemctl", "--user", "daemon-reload")
	}
	return removed, nil
}

// Status reports whether the unit files exist.
func (e Env) Status() (Status, error) {
	files, err := e.Files(time.Hour)
	if err != nil {
		return Status{}, err
	}
	var st Status
	for p := range files {
		if _, err := os.Stat(p); err == nil {
			st.Files = append(st.Files, p)
		}
	}
	st.On = len(st.Files) == len(files)
	return st, nil
}

func (e Env) load(ctx context.Context) error {
	switch e.GOOS {
	case "darwin":
		if out, err := e.Run(ctx, "launchctl", "bootstrap", "gui/"+strconv.Itoa(e.UID), e.plistPath()); err != nil {
			return fmt.Errorf("schedule: launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
		}
	case "linux":
		if out, err := e.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			return fmt.Errorf("schedule: systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if out, err := e.Run(ctx, "systemctl", "--user", "enable", "--now", unitPrefix+".timer"); err != nil {
			return fmt.Errorf("schedule: systemctl enable: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func (e Env) unload(ctx context.Context) error {
	var err error
	switch e.GOOS {
	case "darwin":
		_, err = e.Run(ctx, "launchctl", "bootout", "gui/"+strconv.Itoa(e.UID)+"/"+Label)
	case "linux":
		_, err = e.Run(ctx, "systemctl", "--user", "disable", "--now", unitPrefix+".timer")
	}
	return err
}

func (e Env) plistPath() string {
	return filepath.Join(e.Home, "Library", "LaunchAgents", Label+".plist")
}

func (e Env) systemdDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "systemd", "user")
	}
	return filepath.Join(e.Home, ".config", "systemd", "user")
}

func (e Env) plist(every time.Duration) string {
	x := html.EscapeString
	log := filepath.Join(e.StateDir, "check.log")
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + x(e.Exe) + `</string>
		<string>check</string>
		<string>--notify</string>
	</array>
	<key>StartInterval</key>
	<integer>` + strconv.Itoa(int(every.Seconds())) + `</integer>
	<key>ProcessType</key>
	<string>Background</string>
	<key>LowPriorityIO</key>
	<true/>
	<key>StandardOutPath</key>
	<string>` + x(log) + `</string>
	<key>StandardErrorPath</key>
	<string>` + x(log) + `</string>
</dict>
</plist>
`
}

func (e Env) service() string {
	return `[Unit]
Description=zap: check for stale git worktrees and free disk space

[Service]
Type=oneshot
ExecStart=` + systemdQuote(e.Exe) + ` check --notify
# zap check exits 3 when a limit is passed; that is a finding, not a failure.
SuccessExitStatus=3
Nice=10
IOSchedulingClass=idle
`
}

func timer(every time.Duration) string {
	return `[Unit]
Description=Run zap check periodically

[Timer]
OnBootSec=15min
OnUnitActiveSec=` + strconv.Itoa(int(every.Seconds())) + `s
Persistent=true

[Install]
WantedBy=timers.target
`
}

func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"\\") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t'\"\\$`!*?&;|<>(){}[]~#") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
