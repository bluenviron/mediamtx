package conf

import (
	"fmt"
	"net/url"
	"strings"
)

// PushTarget is a destination to which a path is pushed.
type PushTarget struct {
	URL string `json:"url"`
}

func validatePushTargetURL(rawURL string) (*url.URL, error) {
	replaced := strings.ReplaceAll(rawURL, "$MTX_PATH", "path")
	replaced = strings.ReplaceAll(replaced, "$path", "path")

	return validateURL(replaced)
}

// Validate validates the configuration.
func (p *PushTarget) Validate() error {
	if p.URL == "" {
		return fmt.Errorf("URL is empty")
	}

	u, err := validatePushTargetURL(p.URL)
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

// PushTargets is a list of PushTarget.
type PushTargets []PushTarget

// Validate validates the configuration.
func (p PushTargets) Validate() error {
	seen := make(map[string]struct{})

	for i, target := range p {
		err := target.Validate()
		if err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}

		if _, ok := seen[target.URL]; ok {
			return fmt.Errorf("entry %d: URL is duplicated", i)
		}
		seen[target.URL] = struct{}{}
	}

	return nil
}
