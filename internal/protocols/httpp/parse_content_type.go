package httpp

import "strings"

// ParseContentType parses a Content-Type header and returns the content type.
func ParseContentType(v string) string {
	return strings.TrimSpace(strings.Split(v, ";")[0])
}
