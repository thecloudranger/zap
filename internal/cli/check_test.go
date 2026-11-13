package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/testkit"
)

func TestCheck(t *testing.T) {
	repo, _ := mergedRepo(t)
	home := testkit.TempDir(t)
	tmp := testkit.TempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "claude-"+itoa(os.Getuid()), "-proj", "s1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "claude-"+itoa(os.Getuid()), "-proj", "s1", "x"), make([]byte, 64_000), 0o644))
	var notes []string
	sess := session{t: t, home: home, repo: repo}
	s := struct {
		run func(args ...string) (string, int)
	}{func(args ...string) (string, int) {
		return sess.runWith(func(d *Deps) {
			d.TmpRoot = tmp
			d.Notify = func(_ context.Context, title, body string) bool {
				notes = append(notes, title+": "+body)
				return true
			}
		}, args...)
	}}
	_, code := s.run("roots", "add", filepath.Dir(repo))
	require.Equal(t, ExitOK, code)

	out, code := s.run("check", "--min-stale", "0", "--min-free", "0")
	assert.Equal(t, ExitAttention, code, out)
	assert.Contains(t, out, "1 merged or empty worktree uses")
	assert.Contains(t, out, "free of")
	assert.Contains(t, out, "see zap map")
	assert.Contains(t, out, "Run zap to review them")
	assert.Empty(t, notes, "no notification without --notify")

	_, code = s.run("check", "--min-stale", "0", "--min-free", "0", "--notify")
	assert.Equal(t, ExitAttention, code)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "zap: 1 merged or empty worktree")

	out, code = s.run("check", "--min-stale", "100GB", "--min-free", "0", "--notify")
	assert.Equal(t, ExitOK, code, out)
	assert.Len(t, notes, 1, "nothing to say, no notification")

	out, code = s.run("check", "--min-stale", "100GB", "--min-free", "1PB")
	assert.Equal(t, ExitAttention, code, "a full disk alone is worth flagging")
	assert.NotContains(t, out, "Run zap to review them")

	out, code = s.run("check", "--json", "--min-stale", "0")
	assert.Equal(t, ExitAttention, code)
	var env struct {
		Data CheckResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env), out)
	assert.True(t, env.Data.Attention)
	assert.Equal(t, 1, env.Data.StaleCount)
	assert.Positive(t, env.Data.Leftovers)

	_, code = s.run("check", "--min-stale", "lots")
	assert.Equal(t, ExitUsage, code)
}

func itoa(n int) string { return strconv.Itoa(n) }
