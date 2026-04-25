package hls

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type responseWriterNoCache struct {
	gin.ResponseWriter
}

func (w *responseWriterNoCache) WriteHeader(statusCode int) {
	if statusCode == http.StatusOK {
		w.ResponseWriter.Header().Set("Cache-Control", "private, no-cache")
	}

	w.ResponseWriter.WriteHeader(statusCode)
}
