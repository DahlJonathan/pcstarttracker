package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Config is the agent's persistent state, stored as JSON on disk.
type Config struct {
	ServerURL   string `json:"server_url"`
	DeviceID    string `json:"device_id"`    // persistent machine UUID
	DeviceName  string `json:"device_name"`  // friendly name shown in the app
	DeviceToken string `json:"device_token"` // permanent bearer token (empty until paired)
}

// Paired reports whether onboarding is complete.
func (c *Config) Paired() bool { return c.DeviceToken != "" }

// Dir returns the per-machine config directory (ProgramData on Windows).
func Dir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		// Fallback for non-Windows / dev machines.
		if home, err := os.UserConfigDir(); err == nil {
			base = home
		} else {
			base = "."
		}
	}
	return filepath.Join(base, "PCStatusAgent")
}

func path() string { return filepath.Join(Dir(), "config.json") }

// Load reads config from disk, creating defaults (and a stable device UUID +
// hostname) on first run. It never returns a nil Config on success.
func Load(defaultServerURL string) (*Config, error) {
	cfg := &Config{ServerURL: defaultServerURL}

	data, err := os.ReadFile(path())
	switch {
	case os.IsNotExist(err):
		// First run: generate identity below.
	case err != nil:
		return nil, fmt.Errorf("read config: %w", err)
	default:
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServerURL
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = uuid.NewString()
	}
	if cfg.DeviceName == "" {
		if host, herr := os.Hostname(); herr == nil && host != "" {
			cfg.DeviceName = host
		} else {
			cfg.DeviceName = "My PC"
		}
	}
	return cfg, nil
}

// Save atomically persists the config to disk.
func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path())
}
