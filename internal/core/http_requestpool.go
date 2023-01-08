package core

import (
	"sync"

	"github.com/gin-gonic/gin"
)

type httpRequestPool struct {
	wg sync.WaitGroup
}

func newHTTPRequestPool() *httpRequestPool {
	return &httpRequestPool{}
}

func (rp *httpRequestPool) mw(ctx *gin.Context) {
	rp.wg.Add(1)
	ctx.Next()
	rp.wg.Done()
}

func (rp *httpRequestPool) close() {
	rp.wg.Wait()
}
