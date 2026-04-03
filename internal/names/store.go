package names

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store persists name→port mappings to a JSON file.
// It is safe for concurrent use.
type Store struct {
	path string
	mu   sync.RWMutex
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "requests", "names.json"), nil
}

// New returns a Store backed by path. If path is empty, it uses
// ~/.config/requests/names.json.
func New(path string) (*Store, error) {
	if path == "" {
		var err error
		path, err = defaultPath()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// Lookup returns the port registered for name, or (0, false) if not found.
func (s *Store) Lookup(name string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, err := s.read()
	if err != nil {
		return 0, false
	}
	port, ok := m[name]
	return port, ok
}

// Add registers name→port, overwriting any existing mapping for name.
func (s *Store) Add(name string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, err := s.read()
	if err != nil {
		return err
	}
	m[name] = port
	return s.write(m)
}

// Remove deletes the mapping for name. It is not an error if name does not exist.
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, err := s.read()
	if err != nil {
		return err
	}
	delete(m, name)
	return s.write(m)
}

// List returns a copy of all name→port mappings.
func (s *Store) List() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, err := s.read()
	if err != nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// read loads the JSON file from disk. Caller must hold mu (at least read lock).
func (s *Store) read() (map[string]int, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// write persists m to disk atomically. Caller must hold mu (write lock).
func (s *Store) write(m map[string]int) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file then rename for atomicity.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
