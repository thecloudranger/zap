package policy

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// DisclaimerVersion is bumped when the disclaimer text changes materially,
// which asks every user to accept it again.
const DisclaimerVersion = 1

// Config is zap's global, per-user configuration (config.toml in the config dir).
type Config struct {
	Disclaimer Acceptance `toml:"disclaimer,omitempty"`
	Scan       Scan       `toml:"scan,omitempty"`
}

// Scan configures machine-wide discovery.
type Scan struct {
	// Roots are the folders searched for repositories; empty means the defaults.
	// A leading ~ means the home directory.
	Roots []string `toml:"roots,omitempty"`
}

// Acceptance records that the user accepted the disclaimer.
type Acceptance struct {
	Version    int       `toml:"version,omitempty"`
	AcceptedAt time.Time `toml:"accepted_at,omitzero"`
}

// Accepted reports whether the current disclaimer was accepted.
func (c Config) Accepted() bool { return c.Disclaimer.Version >= DisclaimerVersion }

// LoadConfig reads <dir>/config.toml; a missing file is an empty config.
func LoadConfig(dir string) (Config, error) {
	var c Config
	path := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("policy: %w", err)
	}
	md, err := toml.Decode(string(data), &c)
	if err != nil {
		return c, fmt.Errorf("policy: %s: %w", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return c, fmt.Errorf("policy: %s: unknown key %s", path, und[0])
	}
	return c, nil
}

// SaveConfig writes <dir>/config.toml atomically.
func SaveConfig(dir string, c Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("policy: %w", err)
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return fmt.Errorf("policy: encode config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "config-*.toml")
	if err != nil {
		return fmt.Errorf("policy: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("policy: write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("policy: write config: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, "config.toml")); err != nil {
		return fmt.Errorf("policy: %w", err)
	}
	return nil
}
