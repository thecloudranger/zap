package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/testkit"
)

func TestScheduleCmd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TERMUX_VERSION", "")
	var calls []string
	s := session{t: t, home: testkit.TempDir(t), repo: testkit.TempDir(t)}
	run := func(args ...string) (string, int) {
		return s.runWith(func(d *Deps) {
			d.Exec = func(_ context.Context, name string, args ...string) ([]byte, error) {
				calls = append(calls, name+" "+strings.Join(args, " "))
				return nil, nil
			}
		}, args...)
	}

	out, code := run("schedule")
	require.Equal(t, ExitOK, code, out)
	assert.Contains(t, out, "No schedule")

	out, code = run("schedule", "on", "--every", "6h")
	require.Equal(t, ExitOK, code, out)
	assert.Contains(t, out, "now runs every 6 hours. It never deletes anything.")
	assert.Contains(t, out, "wrote ~/")
	assert.NotEmpty(t, calls)

	out, _ = run("schedule", "status")
	assert.Contains(t, out, "is scheduled")

	out, code = run("schedule", "off")
	require.Equal(t, ExitOK, code, out)
	assert.Contains(t, out, "The schedule is off.")

	_, code = run("schedule", "sometimes")
	assert.Equal(t, ExitUsage, code)
	_, code = run("schedule", "on", "--every", "5m")
	assert.Equal(t, ExitUsage, code)
}

func TestScheduleTermux(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "0.118")
	s := session{t: t, home: testkit.TempDir(t), repo: testkit.TempDir(t)}
	out, code := s.run("schedule", "on")
	require.Equal(t, ExitOK, code, out)
	assert.Contains(t, out, "crontab -e")
	assert.Contains(t, out, "check --notify")
}
