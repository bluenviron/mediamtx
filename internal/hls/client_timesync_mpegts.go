package hls

import (
	"context"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/hls/mpegts"
)

type clientTimeSyncMPEGTS struct {
	startRTC time.Time
	startDTS int64
	td       *mpegts.TimeDecoder
}

func newClientTimeSyncMPEGTS(startDTS int64) *clientTimeSyncMPEGTS {
	return &clientTimeSyncMPEGTS{
		startRTC: time.Now(),
		startDTS: startDTS,
		td:       mpegts.NewTimeDecoder(),
	}
}

func (ts *clientTimeSyncMPEGTS) convertAndSync(ctx context.Context, rawDTS int64, rawPTS int64) (time.Duration, bool) {
	rawDTS = (rawDTS - ts.startDTS) & 0x1FFFFFFFF
	rawPTS = (rawPTS - ts.startDTS) & 0x1FFFFFFFF

	dts := ts.td.Decode(rawDTS)
	pts := ts.td.Decode(rawPTS)

	elapsed := time.Since(ts.startRTC)
	if dts > elapsed {
		select {
		case <-ctx.Done():
			return 0, false
		case <-time.After(dts - elapsed):
		}
	}

	return pts, true
}
