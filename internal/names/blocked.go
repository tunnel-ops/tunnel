package names

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Blocked persists a set of user-blocked ports to JSON.
type Blocked struct {
	path string
}

func defaultBlockedPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "requests", "blocked.json"), nil
}

// NewBlocked returns a Blocked store. If path is empty it uses
// ~/.config/requests/blocked.json.
func NewBlocked(path string) (*Blocked, error) {
	if path == "" {
		var err error
		path, err = defaultBlockedPath()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Blocked{path: path}, nil
}

// Contains reports whether port is in the blocked list.
func (b *Blocked) Contains(port int) bool {
	set, err := b.read()
	if err != nil {
		return false
	}
	_, ok := set[port]
	return ok
}

// Add adds port to the blocked list. No-op if already present.
func (b *Blocked) Add(port int) error {
	set, err := b.read()
	if err != nil {
		return err
	}
	set[port] = struct{}{}
	return b.write(set)
}

// Remove removes port from the blocked list. No-op if not present.
func (b *Blocked) Remove(port int) error {
	set, err := b.read()
	if err != nil {
		return err
	}
	delete(set, port)
	return b.write(set)
}

// List returns all blocked ports in ascending order.
func (b *Blocked) List() []int {
	set, err := b.read()
	if err != nil {
		return nil
	}
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

func (b *Blocked) read() (map[int]struct{}, error) {
	data, err := os.ReadFile(b.path)
	if os.IsNotExist(err) {
		return map[int]struct{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ports []int
	if err := json.Unmarshal(data, &ports); err != nil {
		return nil, err
	}
	set := make(map[int]struct{}, len(ports))
	for _, p := range ports {
		set[p] = struct{}{}
	}
	return set, nil
}

func (b *Blocked) write(set map[int]struct{}) error {
	ports := make([]int, 0, len(set))
	for p := range set {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	data, err := json.MarshalIndent(ports, "", "  ")
	if err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}
