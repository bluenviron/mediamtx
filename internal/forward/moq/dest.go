// Package moq contains the MoQ forward destination.
package moq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	protomoq "github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// Dest is a MoQ forward destination.
type Dest struct {
	Stream          *stream.Stream
	Dest            string
	DestFingerprint string
	Transport       conf.MoQTransport
	Parent          logger.Writer

	mutex         sync.RWMutex
	client        *protomoq.Client
	transport     string
	outboundBytes uint64
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, format, args...)
}

func (d *Dest) addOutboundBytes(n int) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.outboundBytes += uint64(n)
}

// Info returns runtime information.
func (d *Dest) Info() defs.ForwardDestInfo {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if d.transport == "" {
		return defs.ForwardDestInfo{}
	}

	info := defs.ForwardDestInfo{
		OutboundBytes: d.outboundBytes,
		TypeSpecific: &defs.APIForwardDestTypeSpecificMoQ{
			RemoteAddr:    d.client.RemoteAddr().String(),
			Transport:     d.transport,
			OutboundBytes: d.outboundBytes,
		},
	}

	return info
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	u, err := url.Parse(d.Dest)
	if err != nil {
		return err
	}

	transport := d.Transport
	if transport == "" {
		transport = conf.MoQTransportQUIC
	}

	client := &protomoq.Client{
		URL:       u,
		Transport: d.Transport,
		TLSConfig: ptls.MakeConfig(d.DestFingerprint),
		Log:       d,
	}
	err = client.Initialize(ctx)
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	d.mutex.Lock()
	d.client = client
	d.transport = string(transport)
	d.mutex.Unlock()

	defer func() {
		d.mutex.Lock()
		d.client = nil
		d.transport = ""
		d.outboundBytes = 0
		d.mutex.Unlock()
	}()

	r := &stream.Reader{Parent: d}

	cat, setupTracks, err := protomoq.FromStream(d.Stream.OrigDesc)
	if err != nil {
		return err
	}

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return client.Publish(egCtx, ".catalog", 0, nil)
	})

	eg.Go(func() error {
		enc, err2 := json.Marshal(cat)
		if err2 != nil {
			return err2
		}

		return d.writeSubGroup(egCtx, client, 0, 0, false, nil, enc)
	})

	err = eg.Wait()
	if err != nil {
		return err
	}

	groupIDs := make([]uint64, len(setupTracks))

	for i, setupTrack := range setupTracks {
		trackAlias := uint64(i + 1)
		trackName := cat.Tracks[i].Name

		err2 := client.Publish(ctx, trackName, trackAlias, nil)
		if err2 != nil {
			return err2
		}

		setupTrack(r, func(payload []byte, pts int64) error {
			ts := property.Timestamp(pts)
			err3 := d.writeSubGroup(ctx, client, trackAlias, groupIDs[i], true, property.Properties{&ts}, payload)
			if err3 != nil {
				return err3
			}
			groupIDs[i]++
			return nil
		})
	}

	d.Stream.AddReader(r)
	defer d.Stream.RemoveReader(r)

	select {
	case err = <-r.Error():
		return err

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (d *Dest) writeSubGroup(
	ctx context.Context,
	client *protomoq.Client,
	trackAlias uint64,
	groupID uint64,
	_ bool,
	props property.Properties,
	payload []byte,
) error {
	err := client.WriteSubGroup(ctx, trackAlias, groupID, props, payload)
	if err != nil {
		return err
	}

	d.addOutboundBytes(len(payload))
	return nil
}
