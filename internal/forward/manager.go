// Package forward contains stream forwarding utilities.
package forward

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// ErrDestNotFound is returned when a forward destination is not found.
var ErrDestNotFound = errors.New("forward destination not found")

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
	Forward           conf.Forward
	PathManager       PathManager
	Parent            ManagerParent

	mutex        sync.RWMutex
	closed       bool
	destHandlers []*DestHandler
}

// Initialize initializes Manager.
func (m *Manager) Initialize() {
	m.destHandlers = make([]*DestHandler, 0, len(m.Forward))

	for i, dest := range m.Forward {
		m.destHandlers = append(m.destHandlers, m.addDestLocked(dest.Dest, i+1))
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

	destHandlers := m.destHandlers
	m.destHandlers = nil
	m.mutex.Unlock()

	for _, handler := range destHandlers {
		handler.Close()
	}
}

// Log implements logger.Writer.
func (m *Manager) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, "[forward] "+format, args...)
}

func (m *Manager) addDestLocked(dest string, pos int) *DestHandler {
	handler := &DestHandler{
		Dest:              dest,
		ReadTimeout:       m.ReadTimeout,
		WriteTimeout:      m.WriteTimeout,
		UDPMaxPayloadSize: m.UDPMaxPayloadSize,
		PathName:          m.PathName,
		Matches:           m.Matches,
		PathManager:       m.PathManager,
		Parent:            m,
	}
	handler.Initialize(pos)
	return handler
}

// ReloadConf reloads statically-configured destinations.
func (m *Manager) ReloadConf(forwards conf.Forward) {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return
	}

	oldHandlers := make(map[string]*DestHandler, len(m.destHandlers))
	for _, handler := range m.destHandlers {
		oldHandlers[handler.Dest] = handler
	}

	newHandlers := make([]*DestHandler, 0, len(forwards))
	for i, forward := range forwards {
		handler, ok := oldHandlers[forward.Dest]
		if ok {
			delete(oldHandlers, forward.Dest)
			handler.setPos(i + 1)
		} else {
			handler = m.addDestLocked(forward.Dest, i+1)
		}
		newHandlers = append(newHandlers, handler)
	}

	toClose := make([]*DestHandler, 0, len(oldHandlers))
	for _, handler := range oldHandlers {
		toClose = append(toClose, handler)
	}
	m.destHandlers = newHandlers
	m.mutex.Unlock()

	for _, handler := range toClose {
		handler.CloseAsync()
	}
}

// Get gets a destination.
func (m *Manager) Get(id uuid.UUID) (*defs.APIForwardDest, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("terminated")
	}

	for _, handler := range m.destHandlers {
		if handler.ID() == id {
			item := handler.APIItem()
			return &item, nil
		}
	}

	return nil, ErrDestNotFound
}

// List lists all destinations.
func (m *Manager) List() *defs.APIForwardDestList {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	items := make([]defs.APIForwardDest, len(m.destHandlers))
	for i, handler := range m.destHandlers {
		items[i] = handler.APIItem()
	}

	return &defs.APIForwardDestList{Items: items}
}
