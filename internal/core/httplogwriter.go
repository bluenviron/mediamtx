package core

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type httpLogWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *httpLogWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *httpLogWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (w *httpLogWriter) dump() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s %d %s\n", "HTTP/1.1", w.ResponseWriter.Status(), http.StatusText(w.ResponseWriter.Status()))
	w.ResponseWriter.Header().Write(&buf)
	buf.Write([]byte("\n"))
	if w.buf.Len() > 0 {
		fmt.Fprintf(&buf, "(body of %d bytes)", w.buf.Len())
	}
	return buf.String()
}
