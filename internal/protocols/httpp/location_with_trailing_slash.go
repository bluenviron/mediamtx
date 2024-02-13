package httpp

import "net/url"

// LocationWithTrailingSlash returns the URL in a relative format, with a trailing slash.
func LocationWithTrailingSlash(u *url.URL) string {
	l := "./"

	for i := 1; i < len(u.Path); i++ {
		if u.Path[i] == '/' {
			l += "../"
		}
	}

	l += u.Path[1:] + "/"

	if u.RawQuery != "" {
		l += "?" + u.RawQuery
	}

	return l
}
