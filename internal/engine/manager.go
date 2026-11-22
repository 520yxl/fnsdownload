package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Manager struct {
	mu sync.RWMutex
	engines map[string]Engine
}

func NewManager() *Manager { return &Manager{engines: make(map[string]Engine)} }

func (m *Manager) Register(e Engine) error {
	if e == nil || e.Name() == "" { return fmt.Errorf("invalid download engine") }
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.engines[e.Name()]; exists { return fmt.Errorf("engine %q already registered", e.Name()) }
	m.engines[e.Name()] = e
	return nil
}

func (m *Manager) Get(name string) (Engine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.engines[name]
	return e, ok
}

func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.engines))
	for name := range m.engines { names = append(names, name) }
	sort.Strings(names)
	return names
}

func (m *Manager) Initialize(ctx context.Context) map[string]string {
	result := make(map[string]string)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, e := range m.engines {
		if err := e.Initialize(ctx); err != nil { result[name] = err.Error() } else { result[name] = "ready" }
	}
	return result
}

func (m *Manager) Close() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var first error
	for _, e := range m.engines { if err := e.Close(); err != nil && first == nil { first = err } }
	return first
}
