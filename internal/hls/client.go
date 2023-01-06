package hls

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

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
	logger      ClientLogger

	ctx         context.Context
	ctxCancel   func()
	onTracks    func([]format.Format) error
	onData      map[format.Format]func(time.Duration, interface{})
	playlistURL *url.URL

	// out
	outErr chan error
}

// NewClient allocates a Client.
func NewClient(
	playlistURLStr string,
	fingerprint string,
	logger ClientLogger,
) (*Client, error) {
	playlistURL, err := url.Parse(playlistURLStr)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &Client{
		fingerprint: fingerprint,
		logger:      logger,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		playlistURL: playlistURL,
		onData:      make(map[format.Format]func(time.Duration, interface{})),
		outErr:      make(chan error, 1),
	}

	return c, nil
}

// Start starts the client.
func (c *Client) Start() {
	go c.run()
}

// Close closes all the Client resources.
func (c *Client) Close() {
	c.ctxCancel()
}

// Wait waits for any error of the Client.
func (c *Client) Wait() chan error {
	return c.outErr
}

// OnTracks sets a callback that is called when tracks are read.
func (c *Client) OnTracks(cb func([]format.Format) error) {
	c.onTracks = cb
}

// OnData sets a callback that is called when data arrives.
func (c *Client) OnData(forma format.Format, cb func(time.Duration, interface{})) {
	c.onData[forma] = cb
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
		c.onData,
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
