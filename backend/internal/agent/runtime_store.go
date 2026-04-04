package agent

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var ErrRuntimeNotFound = errors.New("agent runtime not found")

type RuntimeStore interface {
	Load() (*RuntimeState, error)
	Save(state *RuntimeState) error
}

type FileRuntimeStore struct {
	path string
	mu   sync.Mutex
}

type RuntimeState struct {
	Agents    []Registration `json:"agents" yaml:"agents"`
	UpdatedAt time.Time      `json:"updatedAt" yaml:"updatedAt"`
}

func NewFileRuntimeStore(path string) *FileRuntimeStore {
	return &FileRuntimeStore{path: path}
}

func (s *FileRuntimeStore) Load() (*RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RuntimeState{}, nil
		}
		return nil, err
	}

	var state RuntimeState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *FileRuntimeStore) Save(state *RuntimeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state == nil {
		state = &RuntimeState{}
	}
	state.UpdatedAt = time.Now().UTC()

	data, err := yaml.Marshal(state)
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
