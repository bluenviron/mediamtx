package push

import (
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// ErrTargetNotFound is returned when a push target is not found.
var ErrTargetNotFound = fmt.Errorf("push target not found")

// ManagerParent is the parent interface.
type ManagerParent interface {
	logger.Writer
}

// Manager manages push targets for a path.
type Manager struct {
	ReadTimeout  conf.Duration
	WriteTimeout conf.Duration
	PathName     string
	Parent       ManagerParent

	mutex   sync.RWMutex
	targets map[uuid.UUID]*Target
	stream  *stream.Stream
}

// Initialize initializes the Manager.
func (m *Manager) Initialize() {
	m.targets = make(map[uuid.UUID]*Target)
}

// Close closes the Manager and all its targets.
func (m *Manager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, t := range m.targets {
		t.Close()
	}
}

// Log implements logger.Writer.
func (m *Manager) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, "[push] "+format, args...)
}

// SetStream sets the stream for all targets.
func (m *Manager) SetStream(strm *stream.Stream) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.stream = strm

	for _, t := range m.targets {
		t.SetStream(strm)
	}
}

// ClearStream clears the stream from all targets.
func (m *Manager) ClearStream() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.stream = nil

	for _, t := range m.targets {
		t.ClearStream()
	}
}

// AddTarget adds a new push target.
func (m *Manager) AddTarget(targetURL string) *Target {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	t := &Target{
		URL:          targetURL,
		ReadTimeout:  m.ReadTimeout,
		WriteTimeout: m.WriteTimeout,
		Parent:       m,
		PathName:     m.PathName,
	}
	t.Initialize()

	if m.stream != nil {
		t.SetStream(m.stream)
	}

	m.targets[t.uuid] = t

	return t
}

// RemoveTarget removes a push target by ID.
func (m *Manager) RemoveTarget(id uuid.UUID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	t, ok := m.targets[id]
	if !ok {
		return ErrTargetNotFound
	}

	t.Close()
	delete(m.targets, id)

	return nil
}

// GetTarget returns a target by ID.
func (m *Manager) GetTarget(id uuid.UUID) (*Target, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	t, ok := m.targets[id]
	if !ok {
		return nil, ErrTargetNotFound
	}

	return t, nil
}

// TargetsList returns a list of all targets.
func (m *Manager) TargetsList() []*Target {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	list := make([]*Target, 0, len(m.targets))
	for _, t := range m.targets {
		list = append(list, t)
	}

	return list
}

// APIItem returns the API list.
func (m *Manager) APIItem() *defs.APIPushTargetList {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	list := &defs.APIPushTargetList{
		Items: make([]*defs.APIPushTarget, 0, len(m.targets)),
	}

	for _, t := range m.targets {
		list.Items = append(list.Items, t.APIItem())
	}

	return list
}
