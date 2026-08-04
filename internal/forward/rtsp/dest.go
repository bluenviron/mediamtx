// Package rtsp contains the RTSP forward destination.
package rtsp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"

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

	dialCtx, dialCtxCancel := context.WithCancel(context.Background())
	defer dialCtxCancel()

	u, err := base.ParseURL(d.Dest)
	if err != nil {
		return err
	}

	dialer := &net.Dialer{}
	client := &gortsplib.Client{
		Scheme:       u.Scheme,
		Host:         u.Host,
		ReadTimeout:  time.Duration(d.ReadTimeout),
		WriteTimeout: time.Duration(d.WriteTimeout),
		DialContext: func(_ context.Context, network string, address string) (net.Conn, error) {
			return dialer.DialContext(dialCtx, network, address)
		},
	}

	err = client.Start()
	if err != nil {
		return err
	}
	defer client.Close()

	terminate := make(chan struct{})

	errChan := make(chan error, 1)
	go func() {
		errChan <- d.runInner(client, desc, terminate)
	}()

	select {
	case err = <-errChan:
		return err

	case <-ctx.Done():
		close(terminate)
		dialCtxCancel()
		client.Close()
		<-errChan
		return fmt.Errorf("terminated")
	}
}

func (d *Dest) runInner(client *gortsplib.Client, desc *description.Session, terminate <-chan struct{}) error {
	u, err := base.ParseURL(d.Dest)
	if err != nil {
		return err
	}

	_, err = client.Announce(u, desc)
	if err != nil {
		return err
	}

	err = client.SetupAll(u, desc.Medias)
	if err != nil {
		return err
	}

	_, err = client.Record()
	if err != nil {
		return err
	}

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
	case err = <-r.Error():
		return err

	case <-terminate:
		return nil
	}
}
