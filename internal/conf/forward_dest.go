package conf

import (
	"fmt"
	"net/url"
	"strings"
)

// ForwardDest is a destination to which a path is forwarded.
type ForwardDest struct {
	Dest            string            `json:"dest"`
	DestFingerprint string            `json:"destFingerprint"`
	MoQTransport    MoQTransport      `json:"moqTransport"`
	WHIPBearerToken string            `json:"whipBearerToken"`
	DestHeaders     map[string]string `json:"destHeaders"`
}

func validateForwardDest(dest string) (*url.URL, error) {
	replaced := strings.ReplaceAll(dest, "$MTX_PATH", "path")

	return validateURL(replaced)
}

// Validate validates the configuration.
func (p *ForwardDest) Validate() error {
	if p.Dest == "" {
		return fmt.Errorf("destination is empty")
	}

	err := checkNotRedacted("whipBearerToken", p.WHIPBearerToken)
	if err != nil {
		return err
	}

	for key, value := range p.DestHeaders {
		err = checkNotRedacted("destHeaders "+key, value)
		if err != nil {
			return err
		}
	}

	err = checkURLNotRedacted("dest", p.Dest)
	if err != nil {
		return err
	}

	u, err := validateForwardDest(p.Dest)
	if err != nil {
		return err
	}

	switch u.Scheme {
	case "rtmp", "rtmps", "rtsp", "rtsps", "rtsp+ws", "rtsps+ws", "srt", "moqt", "whip", "whips":
	default:
		return fmt.Errorf(
			"unsupported scheme '%s', supported ones are "+
				"rtmp, rtmps, rtsp, rtsps, rtsp+ws, rtsps+ws, srt, moqt, whip and whips",
			u.Scheme)
	}

	if len(p.DestHeaders) != 0 && u.Scheme != "rtsp+ws" && u.Scheme != "rtsps+ws" {
		return fmt.Errorf("'destHeaders' can only be used with rtsp+ws and rtsps+ws destinations")
	}

	return nil
}
