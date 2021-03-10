package path

import (
	"sync"

	"github.com/aler9/gortsplib"
)

type reader interface {
	OnIncomingFrame(int, gortsplib.StreamType, []byte)
}

type readersMap struct {
	mutex sync.RWMutex
	ma    map[reader]struct{}
}

func newReadersMap() *readersMap {
	return &readersMap{
		ma: make(map[reader]struct{}),
	}
}

func (m *readersMap) add(reader reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ma[reader] = struct{}{}
}

func (m *readersMap) remove(reader reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.ma, reader)
}

func (m *readersMap) forwardFrame(trackID int, streamType gortsplib.StreamType, buf []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.OnIncomingFrame(trackID, streamType, buf)
	}
}
