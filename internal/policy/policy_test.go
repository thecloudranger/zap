package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/model"
)

func TestParseRepo(t *testing.T) {
	r, err := ParseRepo("")
	require.NoError(t, err, "an empty .zap means defaults")
	assert.Equal(t, Defaults(), r)

	r, err = ParseRepo(Template)
	require.NoError(t, err, "the init template must parse")
	assert.Equal(t, Defaults(), r)

	r, err = ParseRepo(`
target = "develop"
rules = ["merged"]
min_age = "2w"
delete_branch = false
protect = ["release/*"]
`)
	require.NoError(t, err)
	assert.Equal(t, "develop", r.Target)
	assert.True(t, r.Allows(model.StatusMerged))
	assert.False(t, r.Allows(model.StatusEmpty))
	assert.Equal(t, Duration(14*24*time.Hour), r.MinAge)
	assert.False(t, *r.DeleteBranch)
	assert.True(t, r.Protected("release/1.2"))
	assert.False(t, r.Protected("main"), "protect replaces the default list")

	r, err = ParseRepo(`rules = []`)
	require.NoError(t, err)
	assert.Empty(t, r.Rules, "an explicit empty list disables cleaning")
}

func TestParseRepoRejects(t *testing.T) {
	for name, in := range map[string]string{
		"typo key":        `min_agee = "1d"`,
		"unknown status":  `rules = ["merged", "active"]`,
		"bad duration":    `min_age = "soon"`,
		"negative":        `min_age = "-1h"`,
		"bad glob":        `protect = ["[oops"]`,
		"wrong type":      `delete_branch = "yes"`,
		"invalid toml":    `rules = [`,
		"unknown section": "[extra]\nx = 1",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRepo(in)
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestLoadRepo(t *testing.T) {
	dir := t.TempDir()
	_, found, err := LoadRepo(dir)
	require.NoError(t, err)
	assert.False(t, found)

	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(`min_age = "3h"`), 0o644))
	r, found, err := LoadRepo(dir)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, Duration(3*time.Hour), r.MinAge)

	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(`nope = 1`), 0o644))
	_, found, err = LoadRepo(dir)
	assert.True(t, found)
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestConfigRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "zap")
	c, err := LoadConfig(dir)
	require.NoError(t, err)
	assert.False(t, c.Accepted())

	at := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	c.Disclaimer = Acceptance{Version: DisclaimerVersion, AcceptedAt: at}
	require.NoError(t, SaveConfig(dir, c))

	c, err = LoadConfig(dir)
	require.NoError(t, err)
	assert.True(t, c.Accepted())
	assert.True(t, c.Disclaimer.AcceptedAt.Equal(at))
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{"30m": 30 * time.Minute, "12h": 12 * time.Hour, "1d": 24 * time.Hour, "2w": 14 * 24 * time.Hour, "0": 0} {
		got, err := ParseDuration(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := ParseDuration("1.5d")
	assert.Error(t, err)
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "0", 24 * time.Hour: "1d", 14 * 24 * time.Hour: "2w", 3 * time.Hour: "3h", 90 * time.Minute: "90m", time.Second: "1s"} {
		assert.Equal(t, want, FormatDuration(d))
	}
}

func TestKeptIgnored(t *testing.T) {
	d := Defaults()
	got := d.KeptIgnored([]string{"node_modules/", ".env", "config/.env.local", "certs/", "certs/tls.key", "dist/", ".env.example.bak"})
	assert.Equal(t, []string{".env", "config/.env.local", "certs/tls.key", ".env.example.bak"}, got)

	r, err := ParseRepo(`keep_ignored = []`)
	require.NoError(t, err)
	assert.Empty(t, r.KeptIgnored([]string{".env"}), "an empty list turns the check off")
	_, err = ParseRepo(`keep_ignored = ["[bad"]`)
	assert.ErrorIs(t, err, ErrInvalid)
}
