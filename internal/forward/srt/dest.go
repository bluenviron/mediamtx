// Package srt contains the SRT forward destination.
package srt

import (
	"bufio"
	"context"
	"fmt"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func maxPayloadSize(v int) int {
	return ((v - 16) / 188) * 188
}

// Dest is a SRT forward destination.
type Dest struct {
	Stream            *stream.Stream
	Dest              string
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	Parent            logger.Writer

	mutex             sync.RWMutex
	outboundBytesFunc func() uint64
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, "[SRT] "+format, args...)
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
	srtConf := srt.DefaultConfig()
	address, err := srtConf.UnmarshalURL(d.Dest)
	if err != nil {
		return err
	}

	udpMaxPayloadSize := d.UDPMaxPayloadSize
	if udpMaxPayloadSize == 0 {
		udpMaxPayloadSize = 1472
	}
	srtConf.PayloadSize = uint32(maxPayloadSize(udpMaxPayloadSize))

	err = srtConf.Validate()
	if err != nil {
		return err
	}

	conn, err := srt.Dial("srt", address, srtConf)
	if err != nil {
		return err
	}
	defer conn.Close()

	d.mutex.Lock()
	d.outboundBytesFunc = func() uint64 {
		var stats srt.Statistics
		conn.Stats(&stats)
		return stats.Accumulated.ByteSent
	}
	d.mutex.Unlock()

	r := &stream.Reader{Parent: d}
	bw := bufio.NewWriterSize(conn, int(srtConf.PayloadSize))

	err = mpegts.FromStream(d.Stream.OrigDesc, r, bw, conn, time.Duration(d.WriteTimeout))
	if err != nil {
		return err
	}

	d.Stream.AddReader(r)
	defer d.Stream.RemoveReader(r)

	select {
	case readErr := <-r.Error():
		return readErr

	case <-ctx.Done():
		conn.Close()
		return fmt.Errorf("terminated")
	}
}
