// Package rtsp contains the RTSP forward destination.
package rtsp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Dest is a RTSP forward destination.
type Dest struct {
	Stream       *stream.Stream
	Dest         string
	ReadTimeout  conf.Duration
	WriteTimeout conf.Duration
	Parent       logger.Writer

	mutex             sync.RWMutex
	outboundBytesFunc func() uint64
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, "[RTSP] "+format, args...)
}

// OutboundBytes returns the number of bytes sent by the destination.
func (d *Dest) OutboundBytes() uint64 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if d.outboundBytesFunc == nil {
		return 0
	}
	return d.outboundBytesFunc()
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	desc := d.Stream.OutDescCopy()

	dialer := &net.Dialer{}
	client := &gortsplib.Client{
		ReadTimeout:  time.Duration(d.ReadTimeout),
		WriteTimeout: time.Duration(d.WriteTimeout),
		DialContext: func(_ context.Context, network string, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}

	err := client.StartRecording(d.Dest, desc)
	if err != nil {
		return err
	}
	defer client.Close()

	d.mutex.Lock()
	d.outboundBytesFunc = func() uint64 {
		return client.Stats().Session.OutboundBytes
	}
	d.mutex.Unlock()

	r := &stream.Reader{Parent: d}

	for i, media := range d.Stream.OrigDesc.Medias {
		outMedia := desc.Medias[i]

		for _, forma := range media.Formats {
			r.OnData(media, forma, func(u *unit.Unit) error {
				for _, pkt := range u.RTPPackets {
					writeErr := client.WritePacketRTPWithNTP(outMedia, pkt, u.NTP)
					if writeErr != nil {
						return writeErr
					}
				}
				return nil
			})
		}
	}

	d.Stream.AddReader(r)
	defer d.Stream.RemoveReader(r)

	select {
	case readErr := <-r.Error():
		return readErr

	case <-ctx.Done():
		client.Close()
		return fmt.Errorf("terminated")
	}
}
