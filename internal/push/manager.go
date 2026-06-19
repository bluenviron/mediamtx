// Package push contains stream push target utilities.
package push

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

// ErrTargetNotFound is returned when a push target is not found.
var ErrTargetNotFound = errors.New("push target not found")

// ErrTargetAlreadyExists is returned when a push target already exists.
var ErrTargetAlreadyExists = errors.New("push target already exists")

// PathManager is the path manager interface.
type PathManager interface {
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

// ManagerParent is the parent interface.
type ManagerParent interface {
	logger.Writer
}

// Manager manages push targets of a path.
type Manager struct {
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	PathName          string
	Matches           []string
	PathManager       PathManager
	Parent            ManagerParent

	mutex      sync.RWMutex
	closed     bool
	targets    map[uuid.UUID]*Target
	staticURLs map[string]uuid.UUID
}

// Initialize initializes Manager.
func (m *Manager) Initialize(targets conf.PushTargets) {
	m.targets = make(map[uuid.UUID]*Target)
	m.staticURLs = make(map[string]uuid.UUID)

	for _, target := range targets {
		t := m.addTargetLocked(target.URL, defs.APIPushTargetSourceConfig)
		m.staticURLs[target.URL] = t.ID()
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

	targets := make([]*Target, 0, len(m.targets))
	for _, target := range m.targets {
		targets = append(targets, target)
	}
	m.targets = nil
	m.staticURLs = nil
	m.mutex.Unlock()

	for _, target := range targets {
		target.Close()
	}
}

// Log implements logger.Writer.
func (m *Manager) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, "[push] "+format, args...)
}

func (m *Manager) addTargetLocked(rawURL string, source defs.APIPushTargetSource) *Target {
	target := &Target{
		URL:               rawURL,
		Source:            source,
		ReadTimeout:       m.ReadTimeout,
		WriteTimeout:      m.WriteTimeout,
		UDPMaxPayloadSize: m.UDPMaxPayloadSize,
		PathName:          m.PathName,
		Matches:           m.Matches,
		PathManager:       m.PathManager,
		Parent:            m,
	}
	target.Initialize()

	m.targets[target.ID()] = target
	return target
}

func (m *Manager) targetWithURLLocked(rawURL string) *Target {
	for _, target := range m.targets {
		if target.URL == rawURL {
			return target
		}
	}
	return nil
}

// ReloadConf reloads statically-configured targets.
func (m *Manager) ReloadConf(targets conf.PushTargets) {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return
	}

	var toClose []*Target
	newStaticURLs := make(map[string]struct{})
	for _, target := range targets {
		newStaticURLs[target.URL] = struct{}{}
		if _, ok := m.staticURLs[target.URL]; !ok {
			if existing := m.targetWithURLLocked(target.URL); existing != nil {
				delete(m.targets, existing.ID())
				toClose = append(toClose, existing)
			}

			t := m.addTargetLocked(target.URL, defs.APIPushTargetSourceConfig)
			m.staticURLs[target.URL] = t.ID()
		}
	}

	for rawURL, id := range m.staticURLs {
		if _, ok := newStaticURLs[rawURL]; !ok {
			if target, exists := m.targets[id]; exists {
				delete(m.targets, id)
				toClose = append(toClose, target)
			}
			delete(m.staticURLs, rawURL)
		}
	}
	m.mutex.Unlock()

	for _, target := range toClose {
		target.CloseAsync()
	}
}

// Add adds a target.
func (m *Manager) Add(rawURL string) (*Target, error) {
	targetConf := &conf.PushTarget{URL: rawURL}
	err := targetConf.Validate()
	if err != nil {
		return nil, err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return nil, fmt.Errorf("terminated")
	}

	if m.targetWithURLLocked(rawURL) != nil {
		return nil, ErrTargetAlreadyExists
	}

	return m.addTargetLocked(rawURL, defs.APIPushTargetSourceAPI), nil
}

// Remove removes a target.
func (m *Manager) Remove(id uuid.UUID) error {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return fmt.Errorf("terminated")
	}

	target, ok := m.targets[id]
	if !ok {
		m.mutex.Unlock()
		return ErrTargetNotFound
	}

	delete(m.targets, id)
	if target.Source == defs.APIPushTargetSourceConfig {
		delete(m.staticURLs, target.URL)
	}
	m.mutex.Unlock()

	target.CloseAsync()
	return nil
}

// Get gets a target.
func (m *Manager) Get(id uuid.UUID) (*defs.APIPushTarget, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("terminated")
	}

	target, ok := m.targets[id]
	if !ok {
		return nil, ErrTargetNotFound
	}

	item := target.APIItem()
	return &item, nil
}

// List lists all targets.
func (m *Manager) List() *defs.APIPushTargetList {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	items := make([]defs.APIPushTarget, 0, len(m.targets))
	for _, target := range m.targets {
		items = append(items, target.APIItem())
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].URL != items[j].URL {
			return items[i].URL < items[j].URL
		}
		return items[i].ID.String() < items[j].ID.String()
	})

	return &defs.APIPushTargetList{Items: items}
}

// HasAPITargets returns whether the manager contains API-created targets.
func (m *Manager) HasAPITargets() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, target := range m.targets {
		if target.Source == defs.APIPushTargetSourceAPI {
			return true
		}
	}

	return false
}
