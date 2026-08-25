// Package webrtc contains the WebRTC/WHIP forward destination.
package webrtc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
	pwebrtc "github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// Dest is a WebRTC/WHIP forward destination.
type Dest struct {
	Stream          *stream.Stream
	Dest            string
	DestFingerprint string
	ReadTimeout     conf.Duration
	BearerToken     string
	Parent          logger.Writer

	mutex  sync.RWMutex
	client *whip.Client
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, format, args...)
}

// OutboundBytes returns the number of bytes sent by the destination.
func (d *Dest) OutboundBytes() uint64 {
	d.mutex.RLock()
	client := d.client
	d.mutex.RUnlock()

	if client == nil || client.PeerConnection() == nil {
		return 0
	}

	return client.PeerConnection().Stats().BytesSent
}

// RemoteAddr returns the address of the remote peer.
func (d *Dest) RemoteAddr() string {
	d.mutex.RLock()
	client := d.client
	d.mutex.RUnlock()

	if client == nil || client.PeerConnection() == nil {
		return ""
	}

	return client.PeerConnection().RemoteAddr()
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	u, err := url.Parse(d.Dest)
	if err != nil {
		return err
	}

	u.Scheme = strings.Replace(u.Scheme, "whip", "http", 1)

	r := &stream.Reader{Parent: d}
	pc := &pwebrtc.PeerConnection{}

	err = pwebrtc.FromStream(d.Stream.OrigDesc, r, pc)
	if err != nil {
		return err
	}

	tr := &http.Transport{
		TLSClientConfig: ptls.MakeConfig(d.DestFingerprint),
	}
	defer tr.CloseIdleConnections()

	client := &whip.Client{
		URL:                  u,
		Publish:              true,
		OutboundTracks:       pc.OutboundTracks,
		OutboundDataChannels: pc.OutboundDataChannels,
		HTTPClient: &http.Client{
			Timeout:   time.Duration(d.ReadTimeout),
			Transport: tr,
		},
		BearerToken: d.BearerToken,
		Log:         d,
	}
	if err = client.Initialize(ctx); err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	d.mutex.Lock()
	d.client = client
	d.mutex.Unlock()
	defer func() {
		d.mutex.Lock()
		d.client = nil
		d.mutex.Unlock()
	}()

	d.Stream.AddReader(r)
	defer d.Stream.RemoveReader(r)

	clientErr := make(chan error, 1)
	go func() {
		clientErr <- client.Wait()
	}()

	select {
	case err = <-r.Error():
		return err

	case err = <-clientErr:
		return err

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
