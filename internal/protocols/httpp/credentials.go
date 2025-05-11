package httpp

import (
	"net/http"
	"strings"

	"github.com/bluenviron/mediamtx/internal/auth"
)

// Credentials extracts credentials from a HTTP request.
func Credentials(h *http.Request) *auth.Credentials {
	c := &auth.Credentials{}

	for _, auth := range h.Header["Authorization"] {
		if strings.HasPrefix(auth, "Bearer ") {
			// user:pass in Authorization Bearer
			if parts := strings.Split(auth[len("Bearer "):], ":"); len(parts) == 2 {
				c.User = parts[0]
				c.Pass = parts[1]
				return c
			}

			// JWT in Authorization Bearer
			c.Token = auth[len("Bearer "):]
			return c
		}
	}

	// user:pass in Authorization Basic
	c.User, c.Pass, _ = h.BasicAuth()

	return c
}
