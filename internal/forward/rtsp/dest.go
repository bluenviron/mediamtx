// Package rtsp contains the RTSP forward destination.
package rtsp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtsp"
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Dest is a RTSP forward destination.
type Dest struct {
	Stream          *stream.Stream
	Dest            string
	DestFingerprint string
	ReadTimeout     conf.Duration
	WriteTimeout    conf.Duration
	Parent          logger.Writer

	mutex  sync.RWMutex
	client *gortsplib.Client
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, format, args...)
}

// Info returns runtime information.
func (d *Dest) Info() defs.ForwardDestInfo {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if d.client == nil {
		return defs.ForwardDestInfo{}
	}

	stats := d.client.Stats().Session

	transport := ""
	if tr := d.client.Transport(); tr.Session != nil {
		transport = tr.Session.Protocol.String()
	}

	remoteAddr := ""
	if netConn := d.client.NetConn(); netConn != nil {
		remoteAddr = netConn.RemoteAddr().String()
	}

	return defs.ForwardDestInfo{
		OutboundBytes: stats.OutboundBytes,
		TypeSpecific: &defs.APIForwardDestTypeSpecificRTSP{
			RemoteAddr:                     remoteAddr,
			Transport:                      transport,
			InboundBytes:                   stats.InboundBytes,
			InboundRTPPackets:              stats.InboundRTPPackets,
			InboundRTPPacketsLost:          stats.InboundRTPPacketsLost,
			InboundRTPPacketsInError:       stats.InboundRTPPacketsInError,
			InboundRTPPacketsJitter:        stats.InboundRTPPacketsJitter,
			InboundRTCPPackets:             stats.InboundRTCPPackets,
			InboundRTCPPacketsInError:      stats.InboundRTCPPacketsInError,
			OutboundBytes:                  stats.OutboundBytes,
			OutboundRTPPackets:             stats.OutboundRTPPackets,
			OutboundRTPPacketsReportedLost: stats.OutboundRTPPacketsReportedLost,
			OutboundRTCPPackets:            stats.OutboundRTCPPackets,
		},
	}
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	desc := d.Stream.OutDescCopy()

	u, err := base.ParseURL(d.Dest)
	if err != nil {
		return err
	}

	client := &gortsplib.Client{
		Scheme:       u.Scheme,
		Host:         u.Host,
		ReadTimeout:  time.Duration(d.ReadTimeout),
		WriteTimeout: time.Duration(d.WriteTimeout),
		TLSConfig:    ptls.MakeConfig(d.DestFingerprint),
		OnRequest: func(req *base.Request) {
			d.Log(logger.Debug, "[c->s] %v", rtsp.RequestForLog(req))
		},
		OnResponse: func(res *base.Response) {
			d.Log(logger.Debug, "[s->c] %v", res)
		},
		OnTransportSwitch: func(err error) {
			d.Log(logger.Warn, err.Error())
		},
	}

	err = client.Start()
	if err != nil {
		return err
	}

	terminate := make(chan struct{})

	errChan := make(chan error)
	go func() {
		errChan <- d.runInner(client, desc, terminate)
	}()

	select {
	case err = <-errChan:
		client.Close()
		return err

	case <-ctx.Done():
		close(terminate)
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
	d.client = client
	d.mutex.Unlock()

	defer func() {
		d.mutex.Lock()
		d.client = nil
		d.mutex.Unlock()
	}()

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
