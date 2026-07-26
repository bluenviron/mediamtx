// Package forward contains stream forwarding utilities.
package forward

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// ErrDestNotFound is returned when a forward destination is not found.
var ErrDestNotFound = errors.New("forward destination not found")

// ErrDestAlreadyExists is returned when a forward destination already exists.
var ErrDestAlreadyExists = errors.New("forward destination already exists")

// PathManager is the path manager interface.
type PathManager interface {
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

// ManagerParent is the parent interface.
type ManagerParent interface {
	logger.Writer
}

// Manager manages the forward destinations of a path.
type Manager struct {
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	PathName          string
	Matches           []string
	PathManager       PathManager
	Parent            ManagerParent

	mutex        sync.RWMutex
	closed       bool
	destHandlers map[uuid.UUID]*DestHandler
	staticDests  map[string]uuid.UUID
}

// Initialize initializes Manager.
func (m *Manager) Initialize(forwards conf.Forwards) {
	m.destHandlers = make(map[uuid.UUID]*DestHandler)
	m.staticDests = make(map[string]uuid.UUID)

	for _, forward := range forwards {
		handler := m.addDestLocked(forward.Dest, defs.APIForwardSourceConfig)
		m.staticDests[forward.Dest] = handler.ID()
	}
}

// Close closes the manager.
func (m *Manager) Close() {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return
	}

	m.closed = true

	destHandlers := make([]*DestHandler, 0, len(m.destHandlers))
	for _, destHandler := range m.destHandlers {
		destHandlers = append(destHandlers, destHandler)
	}
	m.destHandlers = nil
	m.staticDests = nil
	m.mutex.Unlock()

	for _, handler := range destHandlers {
		handler.CloseAsync()
	}
	for _, handler := range destHandlers {
		handler.Close()
	}
}

// Log implements logger.Writer.
func (m *Manager) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, "[forward] "+format, args...)
}

func (m *Manager) addDestLocked(dest string, source defs.APIForwardSource) *DestHandler {
	handler := &DestHandler{
		Dest:              dest,
		Source:            source,
		ReadTimeout:       m.ReadTimeout,
		WriteTimeout:      m.WriteTimeout,
		UDPMaxPayloadSize: m.UDPMaxPayloadSize,
		PathName:          m.PathName,
		Matches:           m.Matches,
		PathManager:       m.PathManager,
		Parent:            m,
	}
	handler.Initialize()

	m.destHandlers[handler.ID()] = handler
	return handler
}

func (m *Manager) destHandlerWithDestLocked(dest string) *DestHandler {
	for _, handler := range m.destHandlers {
		if handler.Dest == dest {
			return handler
		}
	}
	return nil
}

// ReloadConf reloads statically-configured destinations.
func (m *Manager) ReloadConf(forwards conf.Forwards) {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return
	}

	var toClose []*DestHandler
	newStaticDests := make(map[string]struct{})
	for _, forward := range forwards {
		newStaticDests[forward.Dest] = struct{}{}
		if _, ok := m.staticDests[forward.Dest]; !ok {
			if existing := m.destHandlerWithDestLocked(forward.Dest); existing != nil {
				delete(m.destHandlers, existing.ID())
				toClose = append(toClose, existing)
			}

			handler := m.addDestLocked(forward.Dest, defs.APIForwardSourceConfig)
			m.staticDests[forward.Dest] = handler.ID()
		}
	}

	for dest, id := range m.staticDests {
		if _, ok := newStaticDests[dest]; !ok {
			if handler, exists := m.destHandlers[id]; exists {
				delete(m.destHandlers, id)
				toClose = append(toClose, handler)
			}
			delete(m.staticDests, dest)
		}
	}
	m.mutex.Unlock()

	for _, handler := range toClose {
		handler.CloseAsync()
	}
}

// Add adds a destination.
func (m *Manager) Add(dest string) (*DestHandler, error) {
	forwardConf := &conf.Forward{Dest: dest}
	err := forwardConf.Validate()
	if err != nil {
		return nil, err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return nil, fmt.Errorf("terminated")
	}

	if m.destHandlerWithDestLocked(dest) != nil {
		return nil, ErrDestAlreadyExists
	}

	return m.addDestLocked(dest, defs.APIForwardSourceAPI), nil
}

// Remove removes a destination.
func (m *Manager) Remove(id uuid.UUID) error {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return fmt.Errorf("terminated")
	}

	handler, ok := m.destHandlers[id]
	if !ok {
		m.mutex.Unlock()
		return ErrDestNotFound
	}

	delete(m.destHandlers, id)
	if handler.Source == defs.APIForwardSourceConfig {
		delete(m.staticDests, handler.Dest)
	}
	m.mutex.Unlock()

	handler.CloseAsync()
	return nil
}

// Get gets a destination.
func (m *Manager) Get(id uuid.UUID) (*defs.APIForward, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("terminated")
	}

	handler, ok := m.destHandlers[id]
	if !ok {
		return nil, ErrDestNotFound
	}

	item := handler.APIItem()
	return &item, nil
}

// List lists all destinations.
func (m *Manager) List() *defs.APIForwardList {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	items := make([]defs.APIForward, 0, len(m.destHandlers))
	for _, handler := range m.destHandlers {
		items = append(items, handler.APIItem())
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Dest != items[j].Dest {
			return items[i].Dest < items[j].Dest
		}
		return items[i].ID.String() < items[j].ID.String()
	})

	return &defs.APIForwardList{Items: items}
}

// HasAPIForwards returns whether the manager contains API-created destinations.
func (m *Manager) HasAPIForwards() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, handler := range m.destHandlers {
		if handler.Source == defs.APIForwardSourceAPI {
			return true
		}
	}

	return false
}
