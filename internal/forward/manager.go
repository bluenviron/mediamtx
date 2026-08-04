// Package forward contains stream forwarding utilities.
package forward

import (
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// ErrDestNotFound is returned when a forward destination is not found.
var ErrDestNotFound = errors.New("forward destination not found")

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
	Parent            ManagerParent

	mutex        sync.RWMutex
	destHandlers []*DestHandler
	started      bool
	stream       *stream.Stream
}

// Initialize initializes Manager.
func (m *Manager) Initialize() {
	m.destHandlers = make([]*DestHandler, 0, len(m.Forward))

	for i, dest := range m.Forward {
		destHandler := m.createDestHandler(dest.Dest, i+1)
		m.destHandlers = append(m.destHandlers, destHandler)
	}
}

// Log implements logger.Writer.
func (m *Manager) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, "[forward] "+format, args...)
}

func (m *Manager) createDestHandler(dest string, pos int) *DestHandler {
	handler := &DestHandler{
		Pos:               pos,
		Dest:              dest,
		ReadTimeout:       m.ReadTimeout,
		WriteTimeout:      m.WriteTimeout,
		UDPMaxPayloadSize: m.UDPMaxPayloadSize,
		PathName:          m.PathName,
		Matches:           m.Matches,
		Parent:            m,
	}
	handler.initialize()
	return handler
}

// ReloadConf reloads statically-configured destinations.
func (m *Manager) ReloadConf(forward conf.Forward) {
	m.mutex.Lock()

	newHandlers := make([]*DestHandler, len(forward))
	toClose := make([]*DestHandler, 0)

	for i, dest := range forward {
		if i < len(m.destHandlers) && m.destHandlers[i].Dest == dest.Dest {
			newHandlers[i] = m.destHandlers[i]
		} else {
			if i < len(m.destHandlers) {
				toClose = append(toClose, m.destHandlers[i])
			}

			destHandler := m.createDestHandler(dest.Dest, i+1)
			if m.started {
				destHandler.start(m.stream)
			}

			newHandlers[i] = destHandler
		}
	}

	for i := len(forward); i < len(m.destHandlers); i++ {
		toClose = append(toClose, m.destHandlers[i])
	}

	m.destHandlers = newHandlers

	m.mutex.Unlock()

	if m.started {
		for _, handler := range toClose {
			handler.stop()
		}
	}
}

// Start starts all forward destinations.
func (m *Manager) Start(strm *stream.Stream) {
	m.started = true
	m.stream = strm

	for _, dest := range m.destHandlers {
		dest.start(strm)
	}
}

// Stop stops all forward destinations.
func (m *Manager) Stop() {
	m.started = false

	for _, dest := range m.destHandlers {
		dest.stop()
	}
}

// Get gets a destination.
func (m *Manager) APIGet(id uuid.UUID) (*defs.APIForwardDest, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, handler := range m.destHandlers {
		if handler.ID() == id {
			item := handler.APIItem()
			return &item, nil
		}
	}

	return nil, ErrDestNotFound
}

// List lists all destinations.
func (m *Manager) APIList() *defs.APIForwardDestList {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	items := make([]defs.APIForwardDest, len(m.destHandlers))
	for i, handler := range m.destHandlers {
		items[i] = handler.APIItem()
	}

	return &defs.APIForwardDestList{Items: items}
}
