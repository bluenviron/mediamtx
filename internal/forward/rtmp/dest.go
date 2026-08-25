// Package rtmp contains the RTMP forward destination.
package rtmp

import (
	"context"
	"fmt"
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
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
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

// Dest is a RTMP forward destination.
type Dest struct {
	Stream          *stream.Stream
	Dest            string
	DestFingerprint string
	WriteTimeout    conf.Duration
	Parent          logger.Writer

	mutex             sync.RWMutex
	outboundBytesFunc func() uint64
	remoteAddr        string
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, format, args...)
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

// RemoteAddr returns the address of the remote peer.
func (d *Dest) RemoteAddr() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.remoteAddr
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	u, err := url.Parse(d.Dest)
	if err != nil {
		return err
	}

	conn := &gortmplib.Client{
		URL:       u,
		Publish:   true,
		TLSConfig: ptls.MakeConfig(d.DestFingerprint),
	}

	err = conn.Initialize(ctx)
	if err != nil {
		return err
	}

	terminate := make(chan struct{})

	errChan := make(chan error)
	go func() {
		errChan <- d.runInner(conn, terminate)
	}()

	select {
	case err = <-errChan:
		conn.Close()
		return err

	case <-ctx.Done():
		close(terminate)
		conn.Close()
		<-errChan
		return fmt.Errorf("terminated")
	}
}

func (d *Dest) runInner(conn *gortmplib.Client, terminate <-chan struct{}) error {
	d.mutex.Lock()
	d.outboundBytesFunc = conn.BytesSent
	d.remoteAddr = conn.NetConn().RemoteAddr().String()
	d.mutex.Unlock()

	r := &stream.Reader{Parent: d}
	outDesc := d.Stream.OutDescCopy()

	err := rtmpprotocol.FromStream(
		d.Stream.OrigDesc,
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

	d.Stream.AddReader(r)
	defer d.Stream.RemoveReader(r)

	select {
	case err = <-r.Error():
		return err

	case <-terminate:
		return nil
	}
}
