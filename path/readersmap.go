package path

import (
	"sync"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/base"
)

type Reader interface {
	OnReaderFrame(int, base.StreamType, []byte)
}

type readersMap struct {
	mutex sync.RWMutex
	ma    map[Reader]struct{}
}

func newReadersMap() *readersMap {
	return &readersMap{
		ma: make(map[Reader]struct{}),
	}
}

func (m *readersMap) add(reader Reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ma[reader] = struct{}{}
}

func (m *readersMap) remove(reader Reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.ma, reader)
}

func (m *readersMap) forwardFrame(trackId int, streamType gortsplib.StreamType, buf []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.OnReaderFrame(trackId, streamType, buf)
	}
}
