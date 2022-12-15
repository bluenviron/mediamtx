package core

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"

	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type httpLoggerWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *httpLoggerWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *httpLoggerWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (w *httpLoggerWriter) dump() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s %d %s\n", "HTTP/1.1", w.ResponseWriter.Status(), http.StatusText(w.ResponseWriter.Status()))
	w.ResponseWriter.Header().Write(&buf)
	buf.Write([]byte("\n"))
	if w.buf.Len() > 0 {
		fmt.Fprintf(&buf, "(body of %d bytes)", w.buf.Len())
	}
	return buf.String()
}

type httpLoggerParent interface {
	log(logger.Level, string, ...interface{})
}

func httpLoggerMiddleware(p httpLoggerParent) func(*gin.Context) {
	return func(ctx *gin.Context) {
		p.log(logger.Debug, "[conn %v] %s %s", ctx.ClientIP(), ctx.Request.Method, ctx.Request.URL.Path)

		byts, _ := httputil.DumpRequest(ctx.Request, true)
		p.log(logger.Debug, "[conn %v] [c->s] %s", ctx.ClientIP(), string(byts))

		logw := &httpLoggerWriter{ResponseWriter: ctx.Writer}
		ctx.Writer = logw

		ctx.Writer.Header().Set("Server", "rtsp-simple-server")

		ctx.Next()

		p.log(logger.Debug, "[conn %v] [s->c] %s", ctx.ClientIP(), logw.dump())
	}
}
