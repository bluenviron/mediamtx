package clienthls

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
	newReadPos := r.readPos + len(p)

	curBuf, err := func() ([]byte, error) {
		r.m.mutex.Lock()
		defer r.m.mutex.Unlock()

		if r.m.closed && r.readPos >= r.m.writePos {
			return nil, io.EOF
		}

		for !r.m.closed && newReadPos >= r.m.writePos {
			r.m.cond.Wait()
		}

		return r.m.buf.Bytes(), nil
	}()
	if err != nil {
		return 0, err
	}

	n := copy(p, curBuf[r.readPos:])
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
