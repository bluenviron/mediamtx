package converterhls

import (
	"bytes"
	"io"
	"sync"
)

type multiAccessBufferReader struct {
	m       *multiAccessBuffer
	readPos int
}

func (r *multiAccessBufferReader) Read(p []byte) (int, error) {
	r.m.mutex.Lock()
	defer r.m.mutex.Unlock()

	if r.m.closed && r.m.writePos == r.readPos {
		return 0, io.EOF
	}

	for !r.m.closed && r.m.writePos == r.readPos {
		r.m.cond.Wait()
	}

	buf := r.m.buf.Bytes()
	n := copy(p, buf[r.readPos:])
	r.readPos += n

	return n, nil
}

type multiAccessBuffer struct {
	buf      bytes.Buffer
	closed   bool
	writePos int
	mutex    sync.Mutex
	cond     *sync.Cond
}

func newMultiAccessBuffer() *multiAccessBuffer {
	m := &multiAccessBuffer{}
	m.cond = sync.NewCond(&m.mutex)
	return m
}

func (m *multiAccessBuffer) Close() error {
	m.mutex.Lock()
	m.closed = true
	m.mutex.Unlock()
	m.cond.Broadcast()
	return nil
}

func (m *multiAccessBuffer) Write(p []byte) (int, error) {
	m.mutex.Lock()
	n, _ := m.buf.Write(p)
	m.writePos += n
	m.mutex.Unlock()
	m.cond.Broadcast()
	return n, nil
}

func (m *multiAccessBuffer) NewReader() *multiAccessBufferReader {
	return &multiAccessBufferReader{
		m: m,
	}
}
