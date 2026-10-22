// Package version contains MoQ protocol version constants and utilities.
package version

import (
	"strconv"
	"strings"
)

// Version is a MoQ protocol version.
type Version int

// supported versions, ordered from older to newer.
const (
	Draft16 Version = 16
	Draft17 Version = 17
	Draft18 Version = 18
	Draft19 Version = 19
	Draft20 Version = 20
	Draft21 Version = 21
	Draft22 Version = 22
)

// List contains supported versions, ordered from most preferred to least preferred.
var List = []Version{
	Draft22,
	Draft21,
	Draft20,
	Draft19,
	Draft18,
	Draft17,
	Draft16,
}

// ListAsString is List in string format.
var ListAsString = func() []string {
	list := make([]string, len(List))
	for i, v := range List {
		list[i] = v.String()
	}
	return list
}()

// Parse parses a negotiated MoQ protocol identifier.
func Parse(value string) (Version, bool) {
	text, ok := strings.CutPrefix(value, "moqt-")
	if !ok {
		return 0, false
	}

	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}

	tmp := Version(n)

	switch tmp {
	case Draft16, Draft17, Draft18, Draft19, Draft20, Draft21, Draft22:
		return tmp, true
	default:
		return 0, false
	}
}

// String implements fmt.Stringer.
func (v Version) String() string {
	return "moqt-" + strconv.Itoa(int(v))
}
