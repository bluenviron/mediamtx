package httpserv

import "net/url"

// LocationWithTrailingSlash returns the URL in a relative format, with a trailing slash.
func LocationWithTrailingSlash(u *url.URL) string {
	l := "./" + u.Path[1:] + "/"
	if u.RawQuery != "" {
		l += "?" + u.RawQuery
	}
	return l
}
