package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBin(t *testing.T) {
	dir := t.TempDir()
	realGit := filepath.Join(dir, "git")
	require.NoError(t, os.WriteFile(realGit, []byte("#!/bin/sh\n"), 0o755))
	notExec := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(notExec, []byte("x"), 0o644))

	path := func(p string, err error) func(string) (string, error) {
		return func(string) (string, error) { return p, err }
	}
	find := func(p string, err error) func() (string, error) {
		return func() (string, error) { return p, err }
	}
	boom := errors.New("boom")

	cases := []struct {
		name, goos string
		look       func(string) (string, error)
		find       func() (string, error)
		want       string
	}{
		{"mac launcher swapped for the real git", "darwin", path(macShim, nil), find(realGit, nil), realGit},
		{"linux untouched", "linux", path(macShim, nil), find(realGit, nil), "git"},
		{"homebrew git on PATH kept", "darwin", path("/opt/homebrew/bin/git", nil), find(realGit, nil), "git"},
		{"no git on PATH", "darwin", path("", boom), find(realGit, nil), "git"},
		{"xcrun fails", "darwin", path(macShim, nil), find("", boom), "git"},
		{"xcrun answers the launcher", "darwin", path(macShim, nil), find(macShim, nil), "git"},
		{"xcrun answers a relative path", "darwin", path(macShim, nil), find("bin/git", nil), "git"},
		{"xcrun answers a missing file", "darwin", path(macShim, nil), find(filepath.Join(dir, "nope"), nil), "git"},
		{"xcrun answers a file that can't run", "darwin", path(macShim, nil), find(notExec, nil), "git"},
		{"xcrun answers a folder", "darwin", path(macShim, nil), find(dir, nil), "git"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, resolveBin(c.goos, c.look, c.find))
		})
	}
}
