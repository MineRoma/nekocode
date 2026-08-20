package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Model struct {
	ID               string  `json:"id"`
	ContextWindow    int     `json:"context_window,omitempty"`
	InputPerMillion  float64 `json:"input_per_million,omitempty"`
	OutputPerMillion float64 `json:"output_per_million,omitempty"`
}

type Provider struct {
	Name            string    `json:"name"`
	Compatibility   string    `json:"compatibility"`
	BaseURL         string    `json:"base_url"`
	APIKey          string    `json:"api_key,omitempty"`
	APIKeyEnv       string    `json:"api_key_env,omitempty"`
	Models          []Model   `json:"models,omitempty"`
	ModelsFetchedAt time.Time `json:"models_fetched_at,omitempty"`
}

type Skill struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
	// Mode restricts a skill to one session mode. Empty means every mode.
	Mode    string `json:"mode,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type Config struct {
	ActiveProvider   string     `json:"active_provider"`
	ActiveModel      string     `json:"active_model"`
	Providers        []Provider `json:"providers"`
	Skills           []Skill    `json:"skills,omitempty"`
	AutoCompact      bool       `json:"auto_compact"`
	CompactThreshold float64    `json:"compact_threshold"`
	DefaultContext   int        `json:"default_context_tokens"`
	RefreshHours     int        `json:"model_refresh_hours"`
}

func Default() Config {
	return Config{
		AutoCompact:      true,
		CompactThreshold: 0.90,
		DefaultContext:   128000,
		RefreshHours:     3,
	}
}

type Store struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "neko", "config.json")
	s := &Store{path: path, cfg: Default()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &s.cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	s.applyDefaults()
	return s, nil
}

func (s *Store) applyDefaults() {
	if s.cfg.CompactThreshold <= 0 || s.cfg.CompactThreshold > 1 {
		s.cfg.CompactThreshold = 0.90
	}
	if s.cfg.DefaultContext <= 0 {
		s.cfg.DefaultContext = 128000
	}
	if s.cfg.RefreshHours <= 0 {
		s.cfg.RefreshHours = 3
	}
}

func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.cfg)
}

func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.cfg); err != nil {
		return err
	}
	s.applyDefaults()
	return s.saveLocked()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (c Config) Provider(name string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

func (c Config) ActiveProviderConfig() (Provider, bool) {
	return c.Provider(c.ActiveProvider)
}

func (c Config) ContextWindow() int {
	p, ok := c.ActiveProviderConfig()
	if ok {
		for _, m := range p.Models {
			if m.ID == c.ActiveModel && m.ContextWindow > 0 {
				return m.ContextWindow
			}
		}
	}
	return c.DefaultContext
}

func (c Config) EstimatedCost(usageTokensIn, usageTokensOut int) float64 {
	p, ok := c.ActiveProviderConfig()
	if !ok {
		return 0
	}
	for _, m := range p.Models {
		if m.ID == c.ActiveModel {
			return float64(usageTokensIn)/1_000_000*m.InputPerMillion + float64(usageTokensOut)/1_000_000*m.OutputPerMillion
		}
	}
	return 0
}

func clone(c Config) Config {
	data, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(data, &out)
	return out
}
