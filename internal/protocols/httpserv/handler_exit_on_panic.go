package httpserv

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
)

// exit when there's a panic inside the HTTP handler.
// https://github.com/golang/go/issues/16542
type handlerExitOnPanic struct {
	http.Handler
}

func (h *handlerExitOnPanic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		err := recover()
		if err != nil {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			fmt.Fprintf(os.Stderr, "panic: %v\n\n%s", err, buf[:n])
			os.Exit(1)
		}
	}()
	h.Handler.ServeHTTP(w, r)
}
