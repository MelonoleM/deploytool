package adapter

import (
	"fmt"
	"sync"
)

type Factory func() ServiceAdapter
type Registry struct {
	mu    sync.RWMutex
	types map[string]Factory
}

func NewRegistry() *Registry { return &Registry{types: map[string]Factory{}} }
func (r *Registry) Register(name string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types[name] = f
}
func (r *Registry) Get(name string) (ServiceAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.types[name]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for type %q", name)
	}
	return f(), nil
}
