package whip

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pion/webrtc/v3"
)

// GetICEServers posts a WHIP/WHEP request for ICE servers.
func GetICEServers(
	ctx context.Context,
	hc *http.Client,
	ur string,
) ([]webrtc.ICEServer, error) {
	req, err := http.NewRequestWithContext(ctx, "OPTIONS", ur, nil)
	if err != nil {
		return nil, err
	}

	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return LinkHeaderUnmarshal(res.Header["Link"])
}
