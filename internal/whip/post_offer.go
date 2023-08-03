package whip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/pion/webrtc/v3"
)

// PostOfferResponse is the response to a post offer.
type PostOfferResponse struct {
	Answer   *webrtc.SessionDescription
	Location string
	ETag     string
}

// PostOffer posts a WHIP/WHEP offer.
func PostOffer(
	ctx context.Context,
	hc *http.Client,
	ur string,
	offer *webrtc.SessionDescription,
) (*PostOfferResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", ur, bytes.NewReader([]byte(offer.SDP)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/sdp")

	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/sdp" {
		return nil, fmt.Errorf("bad Content-Type: expected 'application/sdp', got '%s'", contentType)
	}

	acceptPatch := res.Header.Get("Accept-Patch")
	if acceptPatch != "application/trickle-ice-sdpfrag" {
		return nil, fmt.Errorf("wrong Accept-Patch: expected 'application/trickle-ice-sdpfrag', got '%s'", acceptPatch)
	}

	Location := res.Header.Get("Location")

	etag := res.Header.Get("E-Tag")
	if etag == "" {
		return nil, fmt.Errorf("E-Tag is missing")
	}

	sdp, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	answer := &webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(sdp),
	}

	return &PostOfferResponse{
		Answer:   answer,
		Location: Location,
		ETag:     etag,
	}, nil
}
