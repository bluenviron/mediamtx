package udp

import "net/url"

// Params are the parameters of a UDP listener.
type Params struct {
	Address  string
	Source   string
	IntfName string
}

// URLToParams converts a URL to Params.
func URLToParams(u *url.URL) *Params {
	return &Params{
		Address:  u.Host,
		Source:   u.Query().Get("source"),
		IntfName: u.Query().Get("interface"),
	}
}
