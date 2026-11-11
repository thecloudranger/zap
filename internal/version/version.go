// Package version reports build metadata injected with -ldflags -X.
package version

import "runtime/debug"

// Set at build time: -X github.com/thecloudranger/zap/internal/version.Version=v0.1.0
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Info is the build metadata.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
}

// Get returns the ldflags values, falling back to module build info for `go install` builds.
func Get() Info {
	info := Info{Version: Version, Commit: Commit, Date: Date}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	if info.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch {
		case s.Key == "vcs.revision" && info.Commit == "":
			info.Commit = s.Value
		case s.Key == "vcs.time" && info.Date == "":
			info.Date = s.Value
		}
	}
	if len(info.Commit) > 12 {
		info.Commit = info.Commit[:12]
	}
	return info
}
