package httpp

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var errOriginNotAllowed = errors.New("origin not allowed")

func isOriginAllowed(origin string, allowOrigins []string) (string, error) {
	if len(allowOrigins) == 0 {
		return "", errOriginNotAllowed
	}

	for _, o := range allowOrigins {
		if o == "*" {
			return o, nil
		}
	}

	if origin == "" {
		return "", errOriginNotAllowed
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" {
		return "", errOriginNotAllowed
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
			allowedURL.Host == originURL.Host &&
			allowedURL.Port() == originURL.Port() {
			return origin, nil
		}

		if strings.Contains(allowedURL.Host, "*") {
			pattern := strings.ReplaceAll(allowedURL.Host, "*.", "(.*\\.)?")
			pattern = strings.ReplaceAll(pattern, "*", ".*")
			matched, errMatched := regexp.MatchString("^"+pattern+"$", originURL.Host)
			if errMatched == nil && matched {
				return origin, nil
			}
		}
	}

	return "", errOriginNotAllowed
}

// add Access-Control-Allow-Origin and Access-Control-Allow-Credentials
type handlerOrigin struct {
	h            http.Handler
	allowOrigins []string
}

func (h *handlerOrigin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin, err := isOriginAllowed(r.Header.Get("Origin"), h.allowOrigins)
	if err == nil {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	h.h.ServeHTTP(w, r)
}
