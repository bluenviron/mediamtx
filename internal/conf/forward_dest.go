package conf

import (
	"fmt"
	"net/url"
	"strings"
)

// ForwardDest is a destination to which a path is forwarded.
type ForwardDest struct {
	Dest string `json:"dest"`
}

func validateForwardDest(dest string) (*url.URL, error) {
	replaced := strings.ReplaceAll(dest, "$MTX_PATH", "path")
	replaced = strings.ReplaceAll(replaced, "$path", "path")

	return validateURL(replaced)
}

// Validate validates the configuration.
func (p *ForwardDest) Validate() error {
	if p.Dest == "" {
		return fmt.Errorf("destination is empty")
	}

	u, err := validateForwardDest(p.Dest)
	if err != nil {
		return err
	}

	switch u.Scheme {
	case "rtmp", "rtmps", "rtsp", "rtsps", "srt":
	default:
		return fmt.Errorf(
			"unsupported scheme '%s', supported ones are rtmp, rtmps, rtsp, rtsps and srt",
			u.Scheme)
	}

	return nil
}
