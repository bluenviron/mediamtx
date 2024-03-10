package webrtc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/sdp/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// WHIPClient is a WHIP client.
type WHIPClient struct {
	HTTPClient *http.Client
	URL        *url.URL
	Log        logger.Writer

	pc *PeerConnection
}

// Publish publishes tracks.
func (c *WHIPClient) Publish(
	ctx context.Context,
	videoTrack format.Format,
	audioTrack format.Format,
) ([]*OutgoingTrack, error) {
	iceServers, err := WHIPOptionsICEServers(ctx, c.HTTPClient, c.URL.String())
	if err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConf{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
	})
	if err != nil {
		return nil, err
	}

	c.pc = &PeerConnection{
		ICEServers: iceServers,
		API:        api,
		Publish:    true,
		Log:        c.Log,
	}
	err = c.pc.Start()
	if err != nil {
		return nil, err
	}

	tracks, err := c.pc.SetupOutgoingTracks(videoTrack, audioTrack)
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	offer, err := c.pc.CreatePartialOffer()
	if err != nil {
		c.pc.Close()
		return nil, err
	}

	res, err := PostOffer(ctx, c.HTTPClient, c.URL.String(), offer)
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
		WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case ca := <-c.pc.NewLocalCandidate():
			err := WHIPPatchCandidate(ctx, c.HTTPClient, c.URL.String(), offer, res.ETag, ca)
			if err != nil {
				WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
				c.pc.Close()
				return nil, err
			}

		case <-c.pc.GatheringDone():

		case <-c.pc.Connected():
			break outer

		case <-t.C:
			WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
			c.pc.Close()
			return nil, fmt.Errorf("deadline exceeded while waiting connection")
		}
	}

	return tracks, nil
}

// Read reads tracks.
func (c *WHIPClient) Read(ctx context.Context) ([]*IncomingTrack, error) {
	iceServers, err := WHIPOptionsICEServers(ctx, c.HTTPClient, c.URL.String())
	if err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConf{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
	})
	if err != nil {
		return nil, err
	}

	c.pc = &PeerConnection{
		ICEServers: iceServers,
		API:        api,
		Publish:    false,
		Log:        c.Log,
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

	res, err := PostOffer(ctx, c.HTTPClient, c.URL.String(), offer)
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
		WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	// check that there are at most two tracks
	_, err = TrackCount(sdp.MediaDescriptions)
	if err != nil {
		WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	err = c.pc.SetAnswer(res.Answer)
	if err != nil {
		WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case ca := <-c.pc.NewLocalCandidate():
			err := WHIPPatchCandidate(ctx, c.HTTPClient, c.URL.String(), offer, res.ETag, ca)
			if err != nil {
				WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
				c.pc.Close()
				return nil, err
			}

		case <-c.pc.GatheringDone():

		case <-c.pc.Connected():
			break outer

		case <-t.C:
			WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
			c.pc.Close()
			return nil, fmt.Errorf("deadline exceeded while waiting connection")
		}
	}

	tracks, err := c.pc.GatherIncomingTracks(ctx, 0)
	if err != nil {
		WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String()) //nolint:errcheck
		c.pc.Close()
		return nil, err
	}

	return tracks, nil
}

// Close closes the client.
func (c *WHIPClient) Close() error {
	err := WHIPDeleteSession(context.Background(), c.HTTPClient, c.URL.String())
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
