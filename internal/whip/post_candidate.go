// Package whip contains WebRTC / WHIP utilities.
package whip

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/pion/webrtc/v3"
)

// PostCandidate posts a WHIP/WHEP candidate.
func PostCandidate(
	ctx context.Context,
	hc *http.Client,
	ur string,
	offer *webrtc.SessionDescription,
	etag string,
	candidate *webrtc.ICECandidateInit,
) error {
	frag, err := ICEFragmentMarshal(offer.SDP, []*webrtc.ICECandidateInit{candidate})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", ur, bytes.NewReader(frag))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")
	req.Header.Set("If-Match", etag)

	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return nil
}
