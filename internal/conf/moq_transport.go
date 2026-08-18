package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// MoQTransport is the moqTransport parameter.
type MoQTransport string

// supported values.
const (
	MoQTransportQUIC         MoQTransport = "quic"
	MoQTransportWebTransport MoQTransport = "webtransport"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *MoQTransport) UnmarshalJSON(b []byte) error {
	type alias MoQTransport
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case MoQTransportQUIC, MoQTransportWebTransport:

	default:
		return fmt.Errorf("invalid MoQ transport '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *MoQTransport) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
