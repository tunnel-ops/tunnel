package names

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "names.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestAddAndLookup(t *testing.T) {
	s := newTestStore(t)

	if err := s.Add("myapp", 9000); err != nil {
		t.Fatalf("Add: %v", err)
	}

	port, ok := s.Lookup("myapp")
	if !ok || port != 9000 {
		t.Errorf("Lookup(myapp): got (%d, %v), want (9000, true)", port, ok)
	}
}

func TestLookupMissing(t *testing.T) {
	s := newTestStore(t)

	_, ok := s.Lookup("missing")
	if ok {
		t.Error("expected Lookup of unknown name to return false")
	}
}

func TestRemove(t *testing.T) {
	s := newTestStore(t)

	_ = s.Add("api", 8080)
	if err := s.Remove("api"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, ok := s.Lookup("api")
	if ok {
		t.Error("expected Lookup to return false after Remove")
	}
}

func TestRemoveNonExistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Remove("ghost"); err != nil {
		t.Errorf("Remove of non-existent name should not error: %v", err)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	_ = s.Add("a", 3000)
	_ = s.Add("b", 4000)

	list := s.List()
	if len(list) != 2 || list["a"] != 3000 || list["b"] != 4000 {
		t.Errorf("List: got %v", list)
	}
}

func TestEmptyFile(t *testing.T) {
	s := newTestStore(t)
	list := s.List()
	if len(list) != 0 {
		t.Errorf("expected empty list before any adds, got %v", list)
	}
}

func TestOverwrite(t *testing.T) {
	s := newTestStore(t)

	_ = s.Add("svc", 3000)
	_ = s.Add("svc", 4000)

	port, ok := s.Lookup("svc")
	if !ok || port != 4000 {
		t.Errorf("expected overwritten port 4000, got (%d, %v)", port, ok)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "names.json")

	s1, _ := New(path)
	_ = s1.Add("persist", 5000)

	// New store instance reading same file
	s2, _ := New(path)
	port, ok := s2.Lookup("persist")
	if !ok || port != 5000 {
		t.Errorf("expected persisted value 5000, got (%d, %v)", port, ok)
	}
}

func TestAtomicWrite(t *testing.T) {
	s := newTestStore(t)
	_ = s.Add("x", 1234)

	// No .tmp file should remain after write
	if _, err := os.Stat(s.path + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after write")
	}
}
