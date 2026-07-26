package conf

import (
	"fmt"
	"net/url"
	"strings"
)

// Forward is a destination to which a path is forwarded.
type Forward struct {
	Dest string `json:"dest"`
}

func validateForwardDest(dest string) (*url.URL, error) {
	replaced := strings.ReplaceAll(dest, "$MTX_PATH", "path")
	replaced = strings.ReplaceAll(replaced, "$path", "path")

	return validateURL(replaced)
}

// Validate validates the configuration.
func (p *Forward) Validate() error {
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
			"unsupported scheme '%s', supported schemes are rtmp, rtmps, rtsp, rtsps and srt",
			u.Scheme)
	}

	return nil
}

// Forwards is a list of Forward.
type Forwards []Forward

// Validate validates the configuration.
func (p Forwards) Validate() error {
	seen := make(map[string]struct{})

	for i, entry := range p {
		err := entry.Validate()
		if err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}

		if _, ok := seen[entry.Dest]; ok {
			return fmt.Errorf("entry %d: destination is duplicated", i)
		}
		seen[entry.Dest] = struct{}{}
	}

	return nil
}
