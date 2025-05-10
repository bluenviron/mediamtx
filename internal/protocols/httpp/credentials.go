package httpp

import (
	"net/http"
	"strings"

	"github.com/bluenviron/mediamtx/internal/auth"
)

// Credentials extracts credentials from a HTTP request.
func Credentials(h *http.Request) *auth.Credentials {
	c := &auth.Credentials{}

	c.User, c.Pass, _ = h.BasicAuth()

	if auth := h.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if parts := strings.Split(auth[len("Bearer "):], ":"); len(parts) == 2 { // user:pass in Authorization Bearer
			c.User = parts[0]
			c.Pass = parts[1]
		} else { // JWT in Authorization Bearer
			c.Token = auth[len("Bearer "):]
		}
	}

	return c
}
