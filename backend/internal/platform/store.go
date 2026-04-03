package platform

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load() (*PlatformSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadUnlocked()
}

func (s *FileStore) Save(settings *PlatformSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnlocked(settings)
}

func (s *FileStore) SyncRegistry(registryID string) (*PlatformSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	registry := registryByID(settings, registryID)
	if registry != nil {
		now := time.Now().UTC()
		registry.LastSyncedAt = &now
		registry.SyncStatus = "Indexed"
		settings.UpdatedAt = now
		if err := s.saveUnlocked(settings); err != nil {
			return nil, err
		}
		return settings, nil
	}

	return nil, ErrNotFound
}

func (s *FileStore) loadUnlocked() (*PlatformSettings, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			settings := DefaultSettings()
			return &settings, nil
		}
		return nil, err
	}

	var settings PlatformSettings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	normalize(&settings)
	return &settings, nil
}

func (s *FileStore) saveUnlocked(settings *PlatformSettings) error {
	normalize(settings)
	settings.UpdatedAt = time.Now().UTC()

	data, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return os.WriteFile(s.path, data, 0o644)
}

func registryByID(settings *PlatformSettings, registryID string) *OCIRegistry {
	for index := range settings.Registries {
		if strings.TrimSpace(settings.Registries[index].RegistryID) == strings.TrimSpace(registryID) {
			return &settings.Registries[index]
		}
	}
	return nil
}
