package auth

import (
	"net/http"
	"strings"
)

// Credentials is a set of credentials (either user+pass or a token).
type Credentials struct {
	User  string
	Pass  string
	Token string
}

// FromHTTP extracts credentials from a HTTP request.
func (c *Credentials) FromHTTP(h *http.Request) {
	for _, auth := range h.Header["Authorization"] {
		if strings.HasPrefix(auth, "Bearer ") {
			// user:pass in Authorization Bearer
			if parts := strings.Split(auth[len("Bearer "):], ":"); len(parts) == 2 {
				c.User = parts[0]
				c.Pass = parts[1]
				return
			}

			// JWT in Authorization Bearer
			c.Token = auth[len("Bearer "):]
			return
		}
	}

	// user:pass in Authorization Basic
	c.User, c.Pass, _ = h.BasicAuth()
}
