package hls

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

type responseWriterCounter struct {
	gin.ResponseWriter
	bytesSent *atomic.Uint64
}

func (w *responseWriterCounter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytesSent.Add(uint64(n))
	return n, err
}
