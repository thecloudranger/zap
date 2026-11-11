package procs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLsof(t *testing.T) {
	out := []byte("p10\nR1\nczsh\nfcwd\nn/Users/u/dev/app/.claude/worktrees/a\np11\nR10\ncnode\nfcwd\nn/Users/u/dev/app\np12\nR1\ncghost\n")
	ps := ParseLsof(out)
	require.Len(t, ps, 2, "a process without a readable cwd is dropped")
	assert.Equal(t, Proc{PID: 10, PPID: 1, Command: "zsh", Cwd: "/Users/u/dev/app/.claude/worktrees/a"}, ps[0])

	in := Inside(ps, "/Users/u/dev/app/.claude/worktrees/a")
	require.Len(t, in, 1)
	assert.Equal(t, 10, in[0].PID)
	assert.Empty(t, Inside(ps, "/Users/u/dev/app/.claude/worktrees/ab"), "prefix must end at a separator")
	assert.Len(t, Inside(ps, "/Users/u/dev/app"), 2)
}

func TestPPIDFromStat(t *testing.T) {
	assert.Equal(t, 77, ppidFromStat("123 (weird ) name) S 77 123 123 0"))
	assert.Equal(t, 0, ppidFromStat("garbage"))
}

func TestListFindsAChildOfAnotherProcess(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// A process zap did not start: sh starts sleep and exits, so sleep is not our child.
	cmd := exec.Command("sh", "-c", "cd "+dir+" && (sleep 30 >/dev/null 2>&1 & echo $!)")
	out, err := cmd.Output()
	require.NoError(t, err)
	t.Cleanup(func() { _ = exec.Command("kill", string(out[:len(out)-1])).Run() })

	ps, err := List(t.Context())
	require.NoError(t, err)
	in := Inside(ps, dir)
	require.NotEmpty(t, in, "the sleep running in %s is found", dir)
	assert.Equal(t, "sleep", in[0].Command)

	for _, p := range ps {
		assert.NotEqual(t, os.Getpid(), p.PID, "zap itself is excluded")
	}
}

func TestAlive(t *testing.T) {
	assert.True(t, Alive(os.Getpid()))
	assert.False(t, Alive(0))
	cmd := exec.Command("true")
	require.NoError(t, cmd.Run())
	assert.False(t, Alive(cmd.Process.Pid), "an exited, reaped process is gone")
}
