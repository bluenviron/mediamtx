package hls

import (
	"context"
	"sync"
)

type clientRoutinePoolRunnable interface {
	run(context.Context) error
}

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

func (rp *clientRoutinePool) add(r clientRoutinePoolRunnable) {
	rp.wg.Add(1)
	go func() {
		defer rp.wg.Done()

		err := r.run(rp.ctx)
		if err != nil {
			select {
			case rp.err <- err:
			case <-rp.ctx.Done():
			}
		}
	}()
}
