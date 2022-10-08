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
	clientMinDownloadPause             = 5 * time.Second
	clientQueueSize                    = 100
	clientMinSegmentsBeforeDownloading = 2
)

func clientURLAbsolute(base *url.URL, relative string) (*url.URL, error) {
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

	ctx                context.Context
	ctxCancel          func()
	primaryPlaylistURL *url.URL

	// out
	outErr chan error
}

// NewClient allocates a Client.
func NewClient(
	primaryPlaylistURLStr string,
	fingerprint string,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
	logger ClientLogger,
) (*Client, error) {
	primaryPlaylistURL, err := url.Parse(primaryPlaylistURLStr)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &Client{
		fingerprint:        fingerprint,
		onTracks:           onTracks,
		onVideoData:        onVideoData,
		onAudioData:        onAudioData,
		logger:             logger,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		primaryPlaylistURL: primaryPlaylistURL,
		outErr:             make(chan error, 1),
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
	segmentQueue := newClientSegmentQueue()

	dl := newClientDownloader(
		c.primaryPlaylistURL,
		c.fingerprint,
		segmentQueue,
		c.logger,
		c.onTracks,
		c.onVideoData,
		c.onAudioData,
		rp,
	)
	rp.add(dl.run)

	select {
	case err := <-rp.errorChan():
		rp.close()
		return err

	case <-c.ctx.Done():
		rp.close()
		return fmt.Errorf("terminated")
	}
}
