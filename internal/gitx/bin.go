package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// macShim is the git on a stock Mac: a small launcher that asks xcrun where the
// developer tools are and then starts the real git. It costs about 9 ms per call.
const macShim = "/usr/bin/git"

var defaultBin = sync.OnceValue(func() string { return resolveBin(runtime.GOOS, exec.LookPath, xcrunFind) })

// resolveBin returns the git to run. It is "git" from PATH, except that the macOS
// launcher is swapped for the binary it would start, so each call skips the launcher.
// Any doubt falls back to plain "git", which is what zap ran before.
func resolveBin(goos string, lookPath func(string) (string, error), find func() (string, error)) string {
	if goos != "darwin" {
		return "git"
	}
	p, err := lookPath("git")
	if err != nil || p != macShim {
		return "git" // not the launcher (Homebrew, Nix, a wrapper): run what PATH says
	}
	real, err := find()
	if err != nil || !filepath.IsAbs(real) || filepath.Clean(real) == macShim {
		return "git"
	}
	if fi, err := os.Stat(real); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return "git"
	}
	return real
}

// xcrunFind asks xcrun for the git the launcher would start. It honours DEVELOPER_DIR
// and xcode-select, the same way the launcher does.
func xcrunFind() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/xcrun", "-f", "git").Output()
	return strings.TrimSpace(string(out)), err
}
