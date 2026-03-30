package profiles

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type Store interface {
	Load() (*Document, error)
	Save(document *Document) error
}

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load() (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			document := defaultDocument()
			return &document, nil
		}
		return nil, err
	}

	var document Document
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}

	normalizeDocument(&document)
	return &document, nil
}

func (s *FileStore) Save(document *Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeDocument(document)

	data, err := yaml.Marshal(document)
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

type MemoryStore struct {
	mu       sync.Mutex
	document Document
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{document: defaultDocument()}
}

func (s *MemoryStore) Load() (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document := cloneDocument(s.document)
	return &document, nil
}

func (s *MemoryStore) Save(document *Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.document = cloneDocument(*document)
	normalizeDocument(&s.document)
	return nil
}
