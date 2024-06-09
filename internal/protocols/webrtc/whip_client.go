package webrtc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

const (
	webrtcHandshakeTimeout   = 10 * time.Second
	webrtcTrackGatherTimeout = 2 * time.Second
)

// WHIPClient is a WHIP client.
type WHIPClient struct {
	HTTPClient *http.Client
	URL        *url.URL
	Log        logger.Writer

	pc               *PeerConnection
	patchIsSupported bool
}

// Publish publishes tracks.
func (c *WHIPClient) Publish(
	ctx context.Context,
	videoTrack format.Format,
	audioTrack format.Format,
) ([]*OutgoingTrack, error) {
	iceServers, err := c.optionsICEServers(ctx)
	if err != nil {
		return nil, err
	}

	var outgoingTracks []*OutgoingTrack

	if videoTrack != nil {
		outgoingTracks = append(outgoingTracks, &OutgoingTrack{Format: videoTrack})
	}
	if audioTrack != nil {
		outgoingTracks = append(outgoingTracks, &OutgoingTrack{Format: audioTrack})
	}

	c.pc = &PeerConnection{
		ICEServers:         iceServers,
		HandshakeTimeout:   conf.StringDuration(10 * time.Second),
		TrackGatherTimeout: conf.StringDuration(2 * time.Second),
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		Publish:            true,
		OutgoingTracks:     outgoingTracks,
		Log:                c.Log,
	}
	err = c.pc.Start()
	if err != nil {
		return nil, err
	}

	offer, err := c.pc.CreatePartialOffer()
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	res, err := c.postOffer(ctx, offer)
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	c.URL, err = c.URL.Parse(res.Location)
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	err = c.pc.SetAnswer(res.Answer)
	if err != nil {
		c.deleteSession(context.Background()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case ca := <-c.pc.NewLocalCandidate():
			err := c.patchCandidate(ctx, offer, res.ETag, ca)
			if err != nil {
				c.deleteSession(context.Background()) //nolint:errcheck
				c.pc.Close()
				return nil, err
			}

		case <-c.pc.GatheringDone():

		case <-c.pc.Connected():
			break outer

		case <-t.C:
			c.deleteSession(context.Background()) //nolint:errcheck
			c.pc.Close()
			return nil, fmt.Errorf("deadline exceeded while waiting connection")
		}
	}

	return outgoingTracks, nil
}

// Read reads tracks.
func (c *WHIPClient) Read(ctx context.Context) ([]*IncomingTrack, error) {
	iceServers, err := c.optionsICEServers(ctx)
	if err != nil {
		return nil, err
	}

	c.pc = &PeerConnection{
		ICEServers:         iceServers,
		HandshakeTimeout:   conf.StringDuration(10 * time.Second),
		TrackGatherTimeout: conf.StringDuration(2 * time.Second),
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		Publish:            false,
		Log:                c.Log,
	}
	err = c.pc.Start()
	if err != nil {
		return nil, err
	}

	offer, err := c.pc.CreatePartialOffer()
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	res, err := c.postOffer(ctx, offer)
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	c.URL, err = c.URL.Parse(res.Location)
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	var sdp sdp.SessionDescription
	err = sdp.Unmarshal([]byte(res.Answer.SDP))
	if err != nil {
		c.deleteSession(context.Background()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	err = TracksAreValid(sdp.MediaDescriptions)
	if err != nil {
		c.deleteSession(context.Background()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	err = c.pc.SetAnswer(res.Answer)
	if err != nil {
		c.deleteSession(context.Background()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case ca := <-c.pc.NewLocalCandidate():
			err = c.patchCandidate(ctx, offer, res.ETag, ca)
			if err != nil {
				c.deleteSession(context.Background()) //nolint:errcheck
				c.pc.Close()
				return nil, err
			}

		case <-c.pc.GatheringDone():

		case <-c.pc.Connected():
			break outer

		case <-t.C:
			c.deleteSession(context.Background()) //nolint:errcheck
			c.pc.Close()
			return nil, fmt.Errorf("deadline exceeded while waiting connection")
		}
	}

	tracks, err := c.pc.GatherIncomingTracks(ctx)
	if err != nil {
		c.deleteSession(context.Background()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	return tracks, nil
}

// Close closes the client.
func (c *WHIPClient) Close() error {
	err := c.deleteSession(context.Background())
	c.pc.Close()
	return err
}

// Wait waits for client errors.
func (c *WHIPClient) Wait(ctx context.Context) error {
	select {
	case <-c.pc.Disconnected():
		return fmt.Errorf("peer connection closed")

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (c *WHIPClient) optionsICEServers(
	ctx context.Context,
) ([]webrtc.ICEServer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, c.URL.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return LinkHeaderUnmarshal(res.Header["Link"])
}

type whipPostOfferResponse struct {
	Answer   *webrtc.SessionDescription
	Location string
	ETag     string
}

func (c *WHIPClient) postOffer(
	ctx context.Context,
	offer *webrtc.SessionDescription,
) (*whipPostOfferResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL.String(), bytes.NewReader([]byte(offer.SDP)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/sdp")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	contentType := httpp.ParseContentType(req.Header.Get("Content-Type"))
	if contentType != "application/sdp" {
		return nil, fmt.Errorf("bad Content-Type: expected 'application/sdp', got '%s'", contentType)
	}

	c.patchIsSupported = (res.Header.Get("Accept-Patch") == "application/trickle-ice-sdpfrag")

	Location := res.Header.Get("Location")

	etag := res.Header.Get("ETag")
	if etag == "" {
		return nil, fmt.Errorf("ETag is missing")
	}

	sdp, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	answer := &webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(sdp),
	}

	return &whipPostOfferResponse{
		Answer:   answer,
		Location: Location,
		ETag:     etag,
	}, nil
}

func (c *WHIPClient) patchCandidate(
	ctx context.Context,
	offer *webrtc.SessionDescription,
	etag string,
	candidate *webrtc.ICECandidateInit,
) error {
	if !c.patchIsSupported {
		return nil
	}

	frag, err := ICEFragmentMarshal(offer.SDP, []*webrtc.ICECandidateInit{candidate})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.URL.String(), bytes.NewReader(frag))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")
	req.Header.Set("If-Match", etag)

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return nil
}

func (c *WHIPClient) deleteSession(
	ctx context.Context,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.URL.String(), nil)
	if err != nil {
		return err
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return nil
}
