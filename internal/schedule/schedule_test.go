package schedule

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(t *testing.T, goos string) (Env, *[]string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	var calls []string
	return Env{
		GOOS: goos, Home: home, UID: 501, Exe: "/opt/homebrew/bin/zap", StateDir: filepath.Join(home, "state"),
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil
		},
	}, &calls
}

func TestDarwin(t *testing.T) {
	e, calls := env(t, "darwin")
	paths, err := e.On(t.Context(), 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(e.Home, "Library/LaunchAgents/dev.zap.check.plist")}, paths)
	b, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	assert.Contains(t, string(b), "<string>/opt/homebrew/bin/zap</string>")
	assert.Contains(t, string(b), "<string>check</string>\n\t\t<string>--notify</string>")
	assert.Contains(t, string(b), "<integer>86400</integer>")
	assert.NotContains(t, string(b), "clean", "the schedule never deletes")
	assert.Equal(t, []string{
		"launchctl bootout gui/501/dev.zap.check",
		"launchctl bootstrap gui/501 " + paths[0],
	}, *calls)

	st, err := e.Status()
	require.NoError(t, err)
	assert.True(t, st.On)

	removed, err := e.Off(t.Context())
	require.NoError(t, err)
	assert.Equal(t, paths, removed)
	assert.NoFileExists(t, paths[0])
	st, _ = e.Status()
	assert.False(t, st.On)

	removed, err = e.Off(t.Context())
	require.NoError(t, err, "off twice is fine")
	assert.Empty(t, removed)
}

func TestLinux(t *testing.T) {
	e, calls := env(t, "linux")
	e.Exe = "/home/me/go bin/zap"
	paths, err := e.On(t.Context(), 6*time.Hour)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	svc, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/zap-check.service"))
	require.NoError(t, err)
	assert.Contains(t, string(svc), `ExecStart="/home/me/go bin/zap" check --notify`)
	assert.Contains(t, string(svc), "SuccessExitStatus=3")
	tm, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/zap-check.timer"))
	require.NoError(t, err)
	assert.Contains(t, string(tm), "OnUnitActiveSec=21600s")
	assert.Contains(t, *calls, "systemctl --user enable --now zap-check.timer")

	_, err = e.Off(t.Context())
	require.NoError(t, err)
	assert.Contains(t, *calls, "systemctl --user disable --now zap-check.timer")
	assert.NoFileExists(t, filepath.Join(e.Home, ".config/systemd/user/zap-check.timer"))
}

func TestTermux(t *testing.T) {
	e, _ := env(t, "linux")
	e.Termux, e.Exe = true, "/data/data/com.termux/files/usr/bin/zap"
	_, err := e.On(t.Context(), time.Hour)
	assert.ErrorIs(t, err, ErrUnsupported)
	assert.Equal(t, "0 10 * * * /data/data/com.termux/files/usr/bin/zap check --notify", e.CronLine())
	e.Exe = "/tmp/my zap"
	assert.Equal(t, "0 10 * * * '/tmp/my zap' check --notify", e.CronLine())
}
