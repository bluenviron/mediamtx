package httpp

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func isOriginAllowed(origin string, allowOrigins []string) bool {
	if len(allowOrigins) == 0 {
		return false
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" {
		return false
	}

	if originURL.Port() == "" && originURL.Scheme != "" {
		switch originURL.Scheme {
		case "http":
			originURL.Host = net.JoinHostPort(originURL.Host, "80")
		case "https":
			originURL.Host = net.JoinHostPort(originURL.Host, "443")
		}
	}

	for _, o := range allowOrigins {
		if o == "*" {
			return true
		}

		allowedURL, errAllowed := url.Parse(o)
		if errAllowed != nil {
			continue
		}

		if allowedURL.Port() == "" {
			switch allowedURL.Scheme {
			case "http":
				allowedURL.Host = net.JoinHostPort(allowedURL.Host, "80")
			case "https":
				allowedURL.Host = net.JoinHostPort(allowedURL.Host, "443")
			}
		}

		if allowedURL.Scheme == originURL.Scheme &&
			allowedURL.Host == originURL.Host {
			return true
		}

		if allowedURL.Scheme == originURL.Scheme &&
			strings.Contains(allowedURL.Host, "*") {
			pattern := regexp.QuoteMeta(allowedURL.Host)
			pattern = strings.ReplaceAll(pattern, `\*\.`, `(.*\.)?`)
			pattern = strings.ReplaceAll(pattern, `\*`, `.*`)
			matched, errMatched := regexp.MatchString("^"+pattern+"$", originURL.Host)
			if errMatched == nil && matched {
				return true
			}
		}
	}

	return false
}

// add Access-Control-Allow-Origin header.
type handlerOrigin struct {
	h            http.Handler
	allowOrigins []string
}

func (h *handlerOrigin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" {
		if ok := isOriginAllowed(origin, h.allowOrigins); ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
	}

	h.h.ServeHTTP(w, r)
}
