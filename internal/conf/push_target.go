package conf

import (
	"fmt"
	"net/url"
	"strings"
)

// PushTarget is a push target configuration.
type PushTarget struct {
	URL string `json:"url"`
}

// Validate validates a push target.
func (pt *PushTarget) Validate() error {
	if pt.URL == "" {
		return fmt.Errorf("push target URL is empty")
	}

	// Check for valid protocols
	if !strings.HasPrefix(pt.URL, "rtmp://") &&
		!strings.HasPrefix(pt.URL, "rtmps://") &&
		!strings.HasPrefix(pt.URL, "rtsp://") &&
		!strings.HasPrefix(pt.URL, "rtsps://") &&
		!strings.HasPrefix(pt.URL, "srt://") {
		return fmt.Errorf("push target URL must start with rtmp://, rtmps://, rtsp://, rtsps://, or srt://")
	}

	_, err := url.Parse(pt.URL)
	if err != nil {
		return fmt.Errorf("invalid push target URL: %w", err)
	}

	return nil
}

// PushTargets is a list of push targets.
type PushTargets []PushTarget

// Validate validates push targets.
func (pts PushTargets) Validate() error {
	for i, pt := range pts {
		if err := pt.Validate(); err != nil {
			return fmt.Errorf("push target %d: %w", i, err)
		}
	}
	return nil
}
