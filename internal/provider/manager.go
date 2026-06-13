// Package provider: Manager handles the runtime registry of configured
// providers, persistence of API keys/settings, and building live Provider
// instances. It is the single place the daemon and TUI call to add, edit,
// list, test, and switch providers — no technical knowledge required.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is a saved, named provider configuration (one per user-added provider).
type Entry struct {
	ID        string `json:"id"`         // unique slug, e.g. "anthropic", "my-ollama"
	PresetID  string `json:"preset_id"`  // the preset this was based on, or "custom"
	Name      string `json:"name"`       // display name (user may customise)
	Kind      string `json:"kind"`       // "anthropic" | "openai-compatible" | "gemini"
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key,omitempty"` // stored inline if no env var
	KeyEnv    string `json:"key_env,omitempty"` // preferred: read key from env
	IsDefault bool   `json:"is_default"`
}

func (e Entry) Key() string {
	if e.KeyEnv != "" {
		if v := os.Getenv(e.KeyEnv); v != "" {
			return v
		}
	}
	return e.APIKey
}

// Store persists the provider list to disk.
type Store struct {
	mu      sync.RWMutex
	path    string
	entries []Entry
}

func storePath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "velkrogo", "providers.json"), nil
}

// LoadStore loads (or creates) the provider store.
func LoadStore() (*Store, error) {
	p, err := storePath()
	if err != nil {
		return nil, err
	}
	s := &Store{path: p}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, &s.entries)
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

// List returns all configured provider entries.
func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Add adds or replaces a provider entry and saves.
func (s *Store) Add(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, ex := range s.entries {
		if ex.ID == e.ID {
			s.entries[i] = e
			return s.save()
		}
	}
	s.entries = append(s.entries, e)
	return s.save()
}

// Remove deletes an entry by ID.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0]
	for _, e := range s.entries {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	s.entries = kept
	return s.save()
}

// SetDefault marks an entry as the default and clears others.
func (s *Store) SetDefault(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		s.entries[i].IsDefault = s.entries[i].ID == id
	}
	return s.save()
}

// Default returns the default entry, or the first one, or nil.
func (s *Store) Default() *Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		if e.IsDefault {
			cp := e
			return &cp
		}
	}
	if len(s.entries) > 0 {
		cp := s.entries[0]
		return &cp
	}
	return nil
}

// Build constructs a live Provider from an Entry.
func Build(e Entry) (Provider, error) {
	switch e.Kind {
	case "anthropic":
		// import cycle avoided by late import via factory func registered at init
		return buildAnthropicFn(e.Key(), e.BaseURL)
	case "gemini":
		return buildGeminiFn(e.Key())
	case "openai-compatible":
		if e.BaseURL == "" {
			return nil, fmt.Errorf("provider %q: base_url is required", e.ID)
		}
		return buildOpenAICompatFn(e.Name, e.Key(), e.BaseURL)
	default:
		return nil, fmt.Errorf("unknown provider kind %q", e.Kind)
	}
}

// TestConnection does a minimal ping to verify the provider + key work.
func TestConnection(ctx context.Context, e Entry) error {
	p, err := Build(e)
	if err != nil {
		return err
	}
	ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err = p.Chat(ctx2, CompletionRequest{
		Model:     e.Model,
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 5,
	})
	return err
}

// Factory functions registered by adapters at init() to avoid import cycles.
var (
	buildAnthropicFn   func(key, baseURL string) (Provider, error)
	buildGeminiFn      func(key string) (Provider, error)
	buildOpenAICompatFn func(name, key, baseURL string) (Provider, error)
)

// RegisterFactory is called by each adapter package's init().
func RegisterFactory(kind string, fn any) {
	switch kind {
	case "anthropic":
		buildAnthropicFn = fn.(func(string, string) (Provider, error))
	case "gemini":
		buildGeminiFn = fn.(func(string) (Provider, error))
	case "openai-compatible":
		buildOpenAICompatFn = fn.(func(string, string, string) (Provider, error))
	}
}
