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
// Initialize, ReloadConf, Start, Stop are not thread-safe and must all be called from the same goroutine.
func (m *Manager) Initialize() {
	m.destHandlers = make([]*DestHandler, 0, len(m.Forward))

	for i, dest := range m.Forward {
		destHandler := m.createDestHandler(i+1, dest)
		m.destHandlers = append(m.destHandlers, destHandler)
	}
}

// Log implements logger.Writer.
func (m *Manager) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, format, args...)
}

func (m *Manager) createDestHandler(pos int, conf conf.ForwardDest) *DestHandler {
	handler := &DestHandler{
		Pos:               pos,
		Conf:              conf,
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
// Initialize, ReloadConf, Start, Stop are not thread-safe and must all be called from the same goroutine.
func (m *Manager) ReloadConf(forward conf.Forward) {
	m.mutex.Lock()

	reused := make([]bool, len(m.destHandlers))
	newHandlers := make([]*DestHandler, len(forward))

	for i, dest := range forward {
		for j, handler := range m.destHandlers {
			if !reused[j] && handler.Conf == dest {
				reused[j] = true
				handler.setPos(i + 1)
				newHandlers[i] = handler
				break
			}
		}

		if newHandlers[i] == nil {
			destHandler := m.createDestHandler(i+1, dest)
			if m.started {
				destHandler.start(m.stream)
			}

			newHandlers[i] = destHandler
		}
	}

	toClose := make([]*DestHandler, 0)
	for j, handler := range m.destHandlers {
		if !reused[j] {
			toClose = append(toClose, handler)
		}
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
// Initialize, ReloadConf, Start, Stop are not thread-safe and must all be called from the same goroutine.
func (m *Manager) Start(strm *stream.Stream) {
	m.started = true
	m.stream = strm

	for _, dest := range m.destHandlers {
		dest.start(strm)
	}
}

// Stop stops all forward destinations.
// Initialize, ReloadConf, Start, Stop are not thread-safe and must all be called from the same goroutine.
func (m *Manager) Stop() {
	m.started = false

	for _, dest := range m.destHandlers {
		dest.stop()
	}
}

// APIGet gets a forward destination.
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

// APIList lists all forward destinations.
func (m *Manager) APIList() *defs.APIForwardDestList {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	items := make([]defs.APIForwardDest, len(m.destHandlers))
	for i, handler := range m.destHandlers {
		items[i] = handler.APIItem()
	}

	return &defs.APIForwardDestList{Items: items}
}
