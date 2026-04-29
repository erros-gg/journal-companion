// Package config loads and persists the TOML configuration file.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration structure, mirroring config.toml.
type Config struct {
	Network  Network  `toml:"network"`
	Behavior Behavior `toml:"behavior"`
	Watch    []Watch  `toml:"watch"`
}

type Network struct {
	APIBase string `toml:"api_base"`
}

type Behavior struct {
	StartWithOS    bool `toml:"start_with_os"`
	UploadOnChange bool `toml:"upload_on_change"`
	DebounceMs     int  `toml:"debounce_ms"`
}

// Watch describes a single file to monitor and the label the server uses to
// route it to the correct parser. The companion has no opinion on what labels
// mean — that knowledge lives entirely on the server.
type Watch struct {
	Label string `toml:"label"`
	Path  string `toml:"path"`
}

// defaults returns a Config with sane out-of-the-box values.
// NOTE: debounce_ms is set to 2000 rather than the spec's 500.
// ESO writes SavedVariables in multiple bursts on /reloadui and logout;
// 500ms risks triggering mid-write. Users can lower it in config.toml.
func defaults() Config {
	return Config{
		Network: Network{
			APIBase: "https://journal.erros.gg",
		},
		Behavior: Behavior{
			StartWithOS:    false,
			UploadOnChange: true,
			DebounceMs:     2000,
		},
	}
}

// LoadOrCreate reads path; if the file doesn't exist it writes a default
// config and returns it. Subsequent runs read the existing file.
func LoadOrCreate(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := defaults()
		if err := writeDefault(path, &cfg); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		return &cfg, nil
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// Apply defaults for any zero-value fields (tolerates partial configs).
	d := defaults()
	if cfg.Network.APIBase == "" {
		cfg.Network.APIBase = d.Network.APIBase
	}
	if cfg.Behavior.DebounceMs == 0 {
		cfg.Behavior.DebounceMs = d.Behavior.DebounceMs
	}

	return &cfg, nil
}

// Save writes cfg back to path, overwriting the existing file.
// Comments in the original file are lost; use this only for programmatic updates.
func Save(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

// writeDefault writes the initial config with explanatory comments.
func writeDefault(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := `# Journal Companion configuration
# https://github.com/erros-gg/journal-companion
#
# Add one [[watch]] block per file you want to monitor.
# The label tells the server which parser to use — do not rename existing labels.
#
# Example (ESO):
# [[watch]]
# label = "eso-savedvariables"
# path  = 'C:\Users\YourName\Documents\Elder Scrolls Online\live\SavedVariables\Journal.lua'

`
	if _, err := fmt.Fprint(f, header); err != nil {
		return err
	}
	return toml.NewEncoder(f).Encode(cfg)
}
