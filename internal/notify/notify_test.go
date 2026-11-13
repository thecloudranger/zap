package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type call struct {
	name string
	args []string
}

func fake(goos string, env map[string]string, have ...string) (Notifier, *[]call) {
	var calls []call
	return Notifier{
		Run: func(_ context.Context, name string, args ...string) error {
			calls = append(calls, call{name, args})
			return nil
		},
		GOOS: goos,
		LookPath: func(n string) (string, error) {
			for _, h := range have {
				if h == n {
					return "/usr/bin/" + n, nil
				}
			}
			return "", errors.New("not found")
		},
		Getenv: func(k string) string { return env[k] },
	}, &calls
}

func TestSend(t *testing.T) {
	n, calls := fake("darwin", nil, "osascript")
	assert.True(t, n.Send(t.Context(), "zap", `1.2 GB in "stale" worktrees`))
	assert.Equal(t, []call{{"osascript", []string{"-e", `display notification "1.2 GB in \"stale\" worktrees" with title "zap"`}}}, *calls)

	n, calls = fake("linux", map[string]string{"TERMUX_VERSION": "0.118"}, "termux-notification")
	assert.True(t, n.Send(t.Context(), "zap", "body"))
	assert.Equal(t, "termux-notification", (*calls)[0].name)

	n, calls = fake("linux", nil, "notify-send")
	assert.True(t, n.Send(t.Context(), "zap", "$(rm -rf ~)"))
	assert.Equal(t, []string{"--app-name=zap", "zap", "$(rm -rf ~)"}, (*calls)[0].args, "text is an argument, never shell")

	n, calls = fake("linux", nil)
	assert.False(t, n.Send(t.Context(), "zap", "body"), "no tool installed is not an error")
	assert.Empty(t, *calls)
}
