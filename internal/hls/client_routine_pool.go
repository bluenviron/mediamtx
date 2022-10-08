package hls

import (
	"context"
	"sync"
)

type clientRoutinePool struct {
	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup

	err chan error
}

func newClientRoutinePool() *clientRoutinePool {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &clientRoutinePool{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		err:       make(chan error),
	}
}

func (rp *clientRoutinePool) close() {
	rp.ctxCancel()
	rp.wg.Wait()
}

func (rp *clientRoutinePool) errorChan() chan error {
	return rp.err
}

func (rp *clientRoutinePool) add(cb func(context.Context) error) {
	rp.wg.Add(1)
	go func() {
		defer rp.wg.Done()
		select {
		case rp.err <- cb(rp.ctx):
		case <-rp.ctx.Done():
		}
	}()
}
