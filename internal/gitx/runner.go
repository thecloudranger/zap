// Package gitx is the only package that executes git. Everything else talks to it
// through Runner so logic can be tested with canned output.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNotRepo is returned when the directory is not inside a git repository.
var ErrNotRepo = errors.New("not a git repository")

// Runner runs git with args in dir and returns stdout.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Exec runs the real git binary.
type Exec struct {
	Bin string // defaults to "git"
}

// stderrTail bounds how much stderr is kept in error messages.
const stderrTail = 2000

// Run implements Runner. It never takes optional locks, so scanning cannot block a running agent.
func (e Exec) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin := e.Bin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := tail(strings.TrimSpace(stderr.String()), stderrTail)
		if strings.Contains(msg, "not a git repository") {
			return nil, fmt.Errorf("gitx: %s: %w", dir, ErrNotRepo)
		}
		return nil, fmt.Errorf("gitx: git %s: %w: %s", summary(args), err, msg)
	}
	return stdout.Bytes(), nil
}

func summary(args []string) string {
	if len(args) > 2 {
		args = args[:2]
	}
	return strings.Join(args, " ")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
