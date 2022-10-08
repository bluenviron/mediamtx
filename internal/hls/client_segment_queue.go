package hls

import (
	"context"
	"sync"
)

type clientSegmentQueue struct {
	mutex   sync.Mutex
	queue   [][]byte
	didPush chan struct{}
	didPull chan struct{}
}

func newClientSegmentQueue() *clientSegmentQueue {
	return &clientSegmentQueue{
		didPush: make(chan struct{}),
		didPull: make(chan struct{}),
	}
}

func (q *clientSegmentQueue) push(seg []byte) {
	q.mutex.Lock()

	queueWasEmpty := (len(q.queue) == 0)
	q.queue = append(q.queue, seg)

	if queueWasEmpty {
		close(q.didPush)
		q.didPush = make(chan struct{})
	}

	q.mutex.Unlock()
}

func (q *clientSegmentQueue) waitUntilSizeIsBelow(ctx context.Context, n int) bool {
	q.mutex.Lock()

	for len(q.queue) > n {
		q.mutex.Unlock()

		select {
		case <-q.didPull:
		case <-ctx.Done():
			return false
		}

		q.mutex.Lock()
	}

	q.mutex.Unlock()
	return true
}

func (q *clientSegmentQueue) pull(ctx context.Context) ([]byte, bool) {
	q.mutex.Lock()

	for len(q.queue) == 0 {
		didPush := q.didPush
		q.mutex.Unlock()

		select {
		case <-didPush:
		case <-ctx.Done():
			return nil, false
		}

		q.mutex.Lock()
	}

	var seg []byte
	seg, q.queue = q.queue[0], q.queue[1:]

	close(q.didPull)
	q.didPull = make(chan struct{})

	q.mutex.Unlock()
	return seg, true
}
