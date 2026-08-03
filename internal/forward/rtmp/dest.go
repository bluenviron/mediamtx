// Package rtmp contains the RTMP forward destination.
package rtmp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	rtmpprotocol "github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func fourCCToString(c message.FourCC) string {
	return string([]byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)})
}

func fourCCList(desc *description.Session) amf0.StrictArray {
	var videoCount int
	var audioCount int
	var enhanced bool

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			switch forma.(type) {
			case *format.AV1, *format.VP9, *format.H265, *format.Opus, *format.AC3, *format.Generic:
				enhanced = true

			default:
				switch media.Type {
				case description.MediaTypeVideo:
					videoCount++
				case description.MediaTypeAudio:
					audioCount++
				}
			}
		}
	}

	if !enhanced && videoCount <= 1 && audioCount <= 1 {
		return nil
	}

	return amf0.StrictArray{
		fourCCToString(message.FourCCAV1),
		fourCCToString(message.FourCCVP9),
		fourCCToString(message.FourCCHEVC),
		fourCCToString(message.FourCCAVC),
		fourCCToString(message.FourCCOpus),
		fourCCToString(message.FourCCFLAC),
		fourCCToString(message.FourCCAC3),
		fourCCToString(message.FourCCMP4A),
		fourCCToString(message.FourCCMP3),
	}
}

func urlWithDefaultPort(u *url.URL) *url.URL {
	if u.Port() != "" {
		return u
	}

	du := *u
	if u.Scheme == "rtmp" {
		du.Host = net.JoinHostPort(u.Hostname(), "1935")
	} else {
		du.Host = net.JoinHostPort(u.Hostname(), "443")
	}
	return &du
}

// Dest is a RTMP forward destination.
type Dest struct {
	URL          *url.URL
	WriteTimeout conf.Duration
	Parent       logger.Writer

	mutex             sync.RWMutex
	outboundBytesFunc func() uint64
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, "[RTMP] "+format, args...)
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
func (d *Dest) Run(ctx context.Context, strm *stream.Stream) error {
	conn := &gortmplib.Client{
		URL:     urlWithDefaultPort(d.URL),
		Publish: true,
	}

	err := conn.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("connect RTMP destination: %w", err)
	}
	defer conn.Close()

	d.mutex.Lock()
	d.outboundBytesFunc = conn.BytesSent
	d.mutex.Unlock()

	r := &stream.Reader{Parent: d}
	outDesc := strm.OutDescCopy()

	err = rtmpprotocol.FromStream(
		strm.OrigDesc,
		outDesc,
		r,
		conn,
		conn.NetConn(),
		time.Duration(d.WriteTimeout),
		fourCCList(outDesc))
	if err != nil {
		return fmt.Errorf("initialize RTMP destination writer: %w", err)
	}

	conn.NetConn().SetReadDeadline(time.Time{})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	select {
	case readErr := <-r.Error():
		return readErr

	case <-ctx.Done():
		conn.Close()
		return fmt.Errorf("terminated")
	}
}
