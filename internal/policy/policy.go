// Package policy loads the per-repository .zap rules and zap's global config.
// Decoding is strict: an unknown key is an error, so a typo can never loosen a rule.
package policy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/thecloudranger/zap/internal/model"
)

// FileName is the opt-in file at a repository's root.
const FileName = ".zap"

// ErrInvalid wraps every .zap validation failure.
var ErrInvalid = errors.New("invalid .zap")

// Statuses that a .zap may list in rules.
var ruleStatuses = []model.Status{model.StatusMerged, model.StatusEmpty, model.StatusMissing, model.StatusGone}

// defaultRules leave out "gone": a deleted upstream does not prove the work landed.
var defaultRules = []model.Status{model.StatusMerged, model.StatusEmpty, model.StatusMissing}

// Repo is the content of a .zap file.
type Repo struct {
	Target       string         `toml:"target"`
	Rules        []model.Status `toml:"rules"`
	MinAge       Duration       `toml:"min_age"`
	DeleteBranch *bool          `toml:"delete_branch"`
	Protect      []string       `toml:"protect"`
	KeepIgnored  []string       `toml:"keep_ignored"`
}

// Defaults returns the rules used for any key a .zap leaves out.
func Defaults() Repo {
	yes := true
	return Repo{
		Rules:        slices.Clone(defaultRules),
		MinAge:       Duration(24 * time.Hour),
		DeleteBranch: &yes,
		Protect:      []string{"main", "master"},
		KeepIgnored:  []string{".env", ".env.*", ".envrc", "*.pem", "*.key", "*.p12", "*.pfx", "*.tfstate"},
	}
}

// LoadRepo reads <root>/.zap. found is false when the file does not exist,
// which means the repository has not opted in.
func LoadRepo(root string) (r Repo, found bool, err error) {
	path := filepath.Join(root, FileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Repo{}, false, nil
	}
	if err != nil {
		return Repo{}, false, fmt.Errorf("policy: %w", err)
	}
	r, err = ParseRepo(string(data))
	if err != nil {
		return Repo{}, true, fmt.Errorf("policy: %s: %w", path, err)
	}
	return r, true, nil
}

// ParseRepo decodes and validates .zap content, filling defaults.
func ParseRepo(data string) (Repo, error) {
	var r Repo
	md, err := toml.Decode(data, &r)
	if err != nil {
		return Repo{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		return Repo{}, fmt.Errorf("%w: unknown key(s): %s", ErrInvalid, strings.Join(keys, ", "))
	}
	d := Defaults()
	if !md.IsDefined("rules") {
		r.Rules = d.Rules
	}
	if !md.IsDefined("min_age") {
		r.MinAge = d.MinAge
	}
	if r.DeleteBranch == nil {
		r.DeleteBranch = d.DeleteBranch
	}
	if !md.IsDefined("protect") {
		r.Protect = d.Protect
	}
	if !md.IsDefined("keep_ignored") {
		r.KeepIgnored = d.KeepIgnored
	}
	for _, s := range r.Rules {
		if !slices.Contains(ruleStatuses, s) {
			return Repo{}, fmt.Errorf("%w: rules: unknown status %q (allowed: %v)", ErrInvalid, s, ruleStatuses)
		}
	}
	for key, pats := range map[string][]string{"protect": r.Protect, "keep_ignored": r.KeepIgnored} {
		for _, p := range pats {
			if _, err := filepath.Match(p, ""); err != nil {
				return Repo{}, fmt.Errorf("%w: %s: bad pattern %q", ErrInvalid, key, p)
			}
		}
	}
	if r.MinAge < 0 {
		return Repo{}, fmt.Errorf("%w: min_age must not be negative", ErrInvalid)
	}
	return r, nil
}

// Allows reports whether status s is listed in rules.
func (r Repo) Allows(s model.Status) bool { return slices.Contains(r.Rules, s) }

// Protected reports whether branch matches a protect pattern.
func (r Repo) Protected(branch string) bool {
	for _, p := range r.Protect {
		if ok, _ := filepath.Match(p, branch); ok {
			return true
		}
	}
	return false
}

// Template is what `zap init` writes.
const Template = `# zap opt-in file. With this file present, zap may remove worktrees of this repo.
# Every key is optional; the values below are the defaults.

# Branch that "merged" is measured against (default: origin/HEAD, then main, then master).
# target = "main"

# Which statuses zap may remove: "merged", "empty", "missing", and optionally "gone"
# (its remote branch was deleted). For "gone", the worktree is removed but the branch
# is kept unless its work is in the target.
rules = ["merged", "empty", "missing"]

# Never touch a worktree with activity more recent than this (units: m, h, d, w).
min_age = "1d"

# Delete the worktree's branch too. Its tip is saved under refs/zap/backup/ first.
delete_branch = true

# Branches never removed (glob patterns).
protect = ["main", "master"]

# Keep a worktree that holds ignored files with these names: they are not in git, so
# removing the worktree would lose them. Matched against file and folder names.
keep_ignored = [".env", ".env.*", ".envrc", "*.pem", "*.key", "*.p12", "*.pfx", "*.tfstate"]
`

// Duration is a time.Duration that also accepts d (days) and w (weeks) in TOML.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// ParseDuration parses Go durations plus whole-number "Nd" and "Nw".
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if n, ok := strings.CutSuffix(s, suffix); ok {
			v, err := strconv.Atoi(n)
			if err != nil {
				return 0, fmt.Errorf("bad duration %q", s)
			}
			return time.Duration(v) * unit, nil
		}
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	return v, nil
}

// FormatDuration renders d in the units .zap accepts: 1d, 3h, 30m, or 0.
func FormatDuration(d time.Duration) string {
	day := 24 * time.Hour
	switch {
	case d == 0:
		return "0"
	case d%(7*day) == 0:
		return fmt.Sprintf("%dw", d/(7*day))
	case d%day == 0:
		return fmt.Sprintf("%dd", d/day)
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

// KeptIgnored returns the ignored entries whose name matches keep_ignored.
func (r Repo) KeptIgnored(entries []string) []string {
	var hit []string
	for _, e := range entries {
		name := filepath.Base(strings.TrimSuffix(e, "/"))
		for _, p := range r.KeepIgnored {
			if ok, _ := filepath.Match(p, name); ok {
				hit = append(hit, e)
				break
			}
		}
	}
	return hit
}
