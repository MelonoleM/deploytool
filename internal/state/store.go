package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store deliberately uses an atomic JSON file. It replaces the suggested embedded DB
// in this initial dependency-minimal build; the interface keeps a future DB swap local.
type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

func Open(root string) (*Store, error) {
	p := filepath.Join(root, ".deploytool", "state.json")
	s := &Store{path: p, data: Data{Services: map[string]ServiceRecord{}}}
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &s.data); e != nil {
		return nil, fmt.Errorf("read state: %w", e)
	}
	if s.data.Services == nil {
		s.data.Services = map[string]ServiceRecord{}
	}
	return s, nil
}
func (s *Store) Get(n string) (ServiceRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data.Services[n]
	return r, ok
}
func (s *Store) Put(n string, r ServiceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.UpdatedAt = time.Now().UTC()
	s.data.Services[n] = r
	return s.save()
}
func (s *Store) Delete(n string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Services, n)
	return s.save()
}
func (s *Store) Log(action, service, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Operations = append(s.data.Operations, Operation{time.Now().UTC(), action, service, result})
	return s.save()
}
func (s *Store) Purge() error { s.mu.Lock(); defer s.mu.Unlock(); return os.Remove(s.path) }

// Names returns the installed service names in stable order.
func (s *Store) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.data.Services))
	for n := range s.data.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func (s *Store) save() error {
	if e := os.MkdirAll(filepath.Dir(s.path), 0755); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s.data, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
