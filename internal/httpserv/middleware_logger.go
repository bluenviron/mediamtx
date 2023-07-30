package httpserv

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type loggerWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *loggerWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *loggerWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (w *loggerWriter) dump() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s %d %s\n", "HTTP/1.1", w.ResponseWriter.Status(), http.StatusText(w.ResponseWriter.Status()))
	w.ResponseWriter.Header().Write(&buf)
	buf.Write([]byte("\n"))
	if w.buf.Len() > 0 {
		fmt.Fprintf(&buf, "(body of %d bytes)", w.buf.Len())
	}
	return buf.String()
}

// MiddlewareLogger is a middleware that logs requests and responses.
func MiddlewareLogger(p logger.Writer) func(*gin.Context) {
	return func(ctx *gin.Context) {
		p.Log(logger.Debug, "[conn %v] %s %s", ctx.Request.RemoteAddr, ctx.Request.Method, ctx.Request.URL.Path)

		byts, _ := httputil.DumpRequest(ctx.Request, true)
		p.Log(logger.Debug, "[conn %v] [c->s] %s", ctx.Request.RemoteAddr, string(byts))

		logw := &loggerWriter{ResponseWriter: ctx.Writer}
		ctx.Writer = logw

		ctx.Next()

		p.Log(logger.Debug, "[conn %v] [s->c] %s", ctx.Request.RemoteAddr, logw.dump())
	}
}
