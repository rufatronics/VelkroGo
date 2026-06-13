// Package config loads and saves VelkroGo's local configuration. There is no
// default AI provider; a first-run wizard records the user's choice here.
// API keys may be stored inline or referenced via an environment variable
// (key_env), which is preferred.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ProviderConfig struct {
	Kind    string `json:"kind"`     // "anthropic" | "openai-compatible"
	Name    string `json:"name"`     // display name, e.g. "openai", "ollama", "my-vendor"
	BaseURL string `json:"base_url"` // empty = adapter default
	Model   string `json:"model"`
	KeyEnv  string `json:"key_env,omitempty"` // env var holding the API key (preferred)
	APIKey  string `json:"api_key,omitempty"` // inline fallback
}

type Config struct {
	Provider  ProviderConfig `json:"provider"`
	SaverMode bool           `json:"saver_mode"`
}

func (p ProviderConfig) Key() string {
	if p.KeyEnv != "" {
		if v := os.Getenv(p.KeyEnv); v != "" {
			return v
		}
	}
	return p.APIKey
}

// Path returns the per-user config file location (works on Windows and Linux
// via os.UserConfigDir).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "velkrogo", "config.json"), nil
}

var ErrNotConfigured = errors.New("config: no provider configured (run first-time setup)")

func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, ErrNotConfigured
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", p, err)
	}
	if c.Provider.Kind == "" {
		return Config{}, ErrNotConfigured
	}
	return c, nil
}

func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}
