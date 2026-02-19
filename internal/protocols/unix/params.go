package unix

import "net/url"

// Params are the parameters of a unix listener.
type Params struct {
	Path string
}

// URLToParams converts a URL to Params.
func URLToParams(u *url.URL) *Params {
	var pa string
	if u.Path != "" {
		pa = u.Path
	} else {
		pa = u.Host
	}

	return &Params{
		Path: pa,
	}
}
