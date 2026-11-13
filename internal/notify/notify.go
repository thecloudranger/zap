// Package notify shows a desktop notification with the platform's own tool:
// osascript on macOS, termux-notification on Termux, notify-send elsewhere on Linux.
// It never goes through a shell, and a missing tool is not an error: the
// notification is a courtesy on top of zap's printed output and exit code.
package notify

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

// Runner runs a command; tests swap it.
type Runner func(ctx context.Context, name string, args ...string) error

// Exec runs the real command.
func Exec(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// Notifier picks the tool once.
type Notifier struct {
	Run      Runner
	GOOS     string
	LookPath func(string) (string, error)
	Getenv   func(string) string
}

// New returns a Notifier for this machine.
func New() Notifier {
	return Notifier{Run: Exec, GOOS: runtime.GOOS, LookPath: exec.LookPath, Getenv: os.Getenv}
}

// Send shows title and body. It reports whether a notification tool was found and ran.
func (n Notifier) Send(ctx context.Context, title, body string) bool {
	name, args := n.command(title, body)
	if name == "" {
		return false
	}
	if _, err := n.LookPath(name); err != nil {
		return false
	}
	return n.Run(ctx, name, args...) == nil
}

func (n Notifier) command(title, body string) (string, []string) {
	switch {
	case n.GOOS == "darwin":
		// The text travels as AppleScript string literals, quoted here; no shell is involved.
		script := "display notification " + strconv.Quote(body) + " with title " + strconv.Quote(title)
		return "osascript", []string{"-e", script}
	case n.Getenv("TERMUX_VERSION") != "":
		return "termux-notification", []string{"--title", title, "--content", body}
	case n.GOOS == "linux":
		return "notify-send", []string{"--app-name=zap", title, body}
	}
	return "", nil
}
