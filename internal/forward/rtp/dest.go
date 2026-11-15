// Package rtp contains the RTP forward destination.
package rtp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"golang.org/x/net/ipv4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func checkPayloadTypes(desc *description.Session) error {
	used := make(map[uint8]struct{})

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			pt := forma.PayloadType()

			if _, ok := used[pt]; ok {
				return fmt.Errorf("payload type %d is used by more than one format", pt)
			}

			used[pt] = struct{}{}
		}
	}

	return nil
}

func dial(u *url.URL) (net.Conn, error) {
	addr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	intfName := u.Query().Get("interface")

	if intfName != "" {
		if !addr.IP.IsMulticast() {
			conn.Close() //nolint:errcheck
			return nil, fmt.Errorf("parameter 'interface' can be used only with multicast addresses")
		}

		var intf *net.Interface
		intf, err = net.InterfaceByName(intfName)
		if err != nil {
			conn.Close() //nolint:errcheck
			return nil, err
		}

		err = ipv4.NewPacketConn(conn).SetMulticastInterface(intf)
		if err != nil {
			conn.Close() //nolint:errcheck
			return nil, err
		}
	}

	return conn, nil
}

// Dest is a RTP forward destination.
type Dest struct {
	Stream       *stream.Stream
	Dest         string
	WriteTimeout conf.Duration
	Parent       logger.Writer

	outboundBytes atomic.Uint64
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, format, args...)
}

// Info returns runtime information.
func (d *Dest) Info() defs.ForwardDestInfo {
	return defs.ForwardDestInfo{
		OutboundBytes: d.outboundBytes.Load(),
	}
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	desc := d.Stream.OutDescCopy()

	err := checkPayloadTypes(desc)
	if err != nil {
		return err
	}

	u, err := url.Parse(d.Dest)
	if err != nil {
		return err
	}

	conn, err := dial(u)
	if err != nil {
		return err
	}
	defer conn.Close()

	r := &stream.Reader{Parent: d}

	for _, media := range d.Stream.OrigDesc.Medias {
		for _, forma := range media.Formats {
			r.OnData(media, forma, func(u *unit.Unit) error {
				for _, pkt := range u.RTPPackets {
					buf, marshalErr := pkt.Marshal()
					if marshalErr != nil {
						return marshalErr
					}

					conn.SetWriteDeadline(time.Now().Add(time.Duration(d.WriteTimeout))) //nolint:errcheck

					n, writeErr := conn.Write(buf)
					if writeErr != nil {
						return writeErr
					}

					d.outboundBytes.Add(uint64(n))
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

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}
