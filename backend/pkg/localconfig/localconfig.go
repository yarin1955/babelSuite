package localconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Server    string    `json:"server,omitempty"`
	Token     string    `json:"token,omitempty"`
	Email     string    `json:"email,omitempty"`
	FullName  string    `json:"fullName,omitempty"`
	Workspace string    `json:"workspace,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: firstNonEmpty(strings.TrimSpace(path), DefaultConfigPath())}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (*Config, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return &Config{}, nil
	}

	content, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *Store) Save(cfg *Config) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return errors.New("config path is empty")
	}
	if cfg == nil {
		cfg = &Config{}
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(s.path, body, 0o600)
}

func (s *Store) ClearSession() error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}

	cfg.Token = ""
	cfg.Email = ""
	cfg.FullName = ""
	cfg.Workspace = ""
	cfg.ExpiresAt = time.Time{}
	return s.Save(cfg)
}

func DefaultConfigPath() string {
	if envPath := strings.TrimSpace(os.Getenv("BABELSUITE_CONFIG")); envPath != "" {
		return envPath
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".babelsuite", "config.json")
	}

	return filepath.Join(home, ".babelsuite", "config.json")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
