// Package procs lists the working directories of running processes, to tell whether a
// worktree is in use by an editor, shell or agent.
package procs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Proc is a process and its current working directory.
type Proc struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

// ErrUnavailable is returned when process working directories cannot be read.
var ErrUnavailable = errors.New("cannot list process working directories")

// List returns every readable process's working directory, excluding zap itself and the
// processes it started (such as the git commands it runs).
func List(ctx context.Context) ([]Proc, error) {
	var (
		all []Proc
		err error
	)
	if runtime.GOOS == "linux" {
		all, err = fromProc("/proc")
	} else {
		all, err = fromLsof(ctx)
	}
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	out := all[:0]
	for _, p := range all {
		if p.PID != self && p.PPID != self {
			out = append(out, p)
		}
	}
	return out, nil
}

// Inside returns the processes whose working directory is dir or below it.
func Inside(ps []Proc, dir string) []Proc {
	var in []Proc
	dir = filepath.Clean(dir)
	for _, p := range ps {
		if p.Cwd == dir || strings.HasPrefix(p.Cwd, dir+string(filepath.Separator)) {
			in = append(in, p)
		}
	}
	return in
}

// fromLsof asks lsof for every process's cwd in one call (macOS, BSD).
func fromLsof(ctx context.Context) ([]Proc, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsof", "-a", "-d", "cwd", "-FpRcn")
	cmd.WaitDelay = 2 * time.Second
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	// lsof exits 1 when some processes could not be inspected; its output is still valid.
	var ee *exec.ExitError
	partial := errors.As(err, &ee) && stdout.Len() > 0
	if err != nil && !partial {
		return nil, fmt.Errorf("procs: %w: lsof: %w", ErrUnavailable, err)
	}
	return ParseLsof(stdout.Bytes()), nil
}

// ParseLsof parses `lsof -FpRcn` output: one field per line, keyed by its first byte.
func ParseLsof(data []byte) []Proc {
	var (
		out []Proc
		cur *Proc
	)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		val := line[1:]
		switch line[0] {
		case 'p':
			pid, _ := strconv.Atoi(val)
			out = append(out, Proc{PID: pid})
			cur = &out[len(out)-1]
		case 'R':
			if cur != nil {
				cur.PPID, _ = strconv.Atoi(val)
			}
		case 'c':
			if cur != nil {
				cur.Command = val
			}
		case 'n':
			if cur != nil {
				cur.Cwd = val
			}
		}
	}
	keep := out[:0]
	for _, p := range out {
		if p.Cwd != "" {
			keep = append(keep, p)
		}
	}
	return keep
}

// fromProc reads /proc/<pid>/cwd links (Linux, Android). Processes owned by other users
// are unreadable and skipped.
func fromProc(root string) ([]Proc, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("procs: %w: %w", ErrUnavailable, err)
	}
	var out []Proc
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(root, e.Name())
		cwd, err := os.Readlink(filepath.Join(dir, "cwd"))
		if err != nil {
			continue
		}
		p := Proc{PID: pid, Cwd: cwd}
		if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			p.Command = strings.TrimSpace(string(comm))
		}
		if stat, err := os.ReadFile(filepath.Join(dir, "stat")); err == nil {
			p.PPID = ppidFromStat(string(stat))
		}
		out = append(out, p)
	}
	return out, nil
}

// ppidFromStat reads field 4 of /proc/<pid>/stat; the command in field 2 may contain
// spaces and parentheses, so parsing starts after its last ')'.
func ppidFromStat(stat string) int {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(stat[i+1:])
	if len(f) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(f[1])
	return ppid
}

// Alive reports whether a process with pid exists (it may belong to another user).
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return signalZero(p)
}
