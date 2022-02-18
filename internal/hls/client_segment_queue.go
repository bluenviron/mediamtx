package hls

import (
	"context"
	"fmt"
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

func (q *clientSegmentQueue) waitUntilSizeIsBelow(ctx context.Context, n int) {
	q.mutex.Lock()

	for len(q.queue) > n {
		q.mutex.Unlock()

		select {
		case <-q.didPull:
		case <-ctx.Done():
			return
		}

		q.mutex.Lock()
	}

	q.mutex.Unlock()
}

func (q *clientSegmentQueue) waitAndPull(ctx context.Context) ([]byte, error) {
	q.mutex.Lock()

	for len(q.queue) == 0 {
		didPush := q.didPush
		q.mutex.Unlock()

		select {
		case <-didPush:
		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}

		q.mutex.Lock()
	}

	var seg []byte
	seg, q.queue = q.queue[0], q.queue[1:]

	close(q.didPull)
	q.didPull = make(chan struct{})

	q.mutex.Unlock()
	return seg, nil
}
