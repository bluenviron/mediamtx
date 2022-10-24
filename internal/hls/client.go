package hls

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	clientMPEGTSEntryQueueSize        = 100
	clientFMP4MaxPartTracksPerSegment = 200
	clientLiveStartingInvPosition     = 3
	clientLiveMaxInvPosition          = 5
	clientMaxDTSRTCDiff               = 10 * time.Second
)

func clientAbsoluteURL(base *url.URL, relative string) (*url.URL, error) {
	u, err := url.Parse(relative)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(u), nil
}

// ClientLogger allows to receive log lines.
type ClientLogger interface {
	Log(level logger.Level, format string, args ...interface{})
}

// Client is a HLS client.
type Client struct {
	fingerprint string
	onTracks    func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error
	onVideoData func(time.Duration, [][]byte)
	onAudioData func(time.Duration, []byte)
	logger      ClientLogger

	ctx         context.Context
	ctxCancel   func()
	playlistURL *url.URL

	// out
	outErr chan error
}

// NewClient allocates a Client.
func NewClient(
	playlistURLStr string,
	fingerprint string,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
	logger ClientLogger,
) (*Client, error) {
	playlistURL, err := url.Parse(playlistURLStr)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &Client{
		fingerprint: fingerprint,
		onTracks:    onTracks,
		onVideoData: onVideoData,
		onAudioData: onAudioData,
		logger:      logger,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		playlistURL: playlistURL,
		outErr:      make(chan error, 1),
	}

	go c.run()

	return c, nil
}

// Close closes all the Client resources.
func (c *Client) Close() {
	c.ctxCancel()
}

// Wait waits for any error of the Client.
func (c *Client) Wait() chan error {
	return c.outErr
}

func (c *Client) run() {
	c.outErr <- c.runInner()
}

func (c *Client) runInner() error {
	rp := newClientRoutinePool()

	dl := newClientDownloaderPrimary(
		c.playlistURL,
		c.fingerprint,
		c.logger,
		rp,
		c.onTracks,
		c.onVideoData,
		c.onAudioData,
	)
	rp.add(dl)

	select {
	case err := <-rp.errorChan():
		rp.close()
		return err

	case <-c.ctx.Done():
		rp.close()
		return fmt.Errorf("terminated")
	}
}
