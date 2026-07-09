package stream

import (
	"crypto/rand"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/ntpestimator"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func unitSize(u *unit.Unit) uint64 {
	n := uint64(0)
	for _, pkt := range u.RTPPackets {
		n += uint64(pkt.MarshalSize())
	}
	return n
}

func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

type streamFormat struct {
	origFormat           format.Format
	alwaysAvailable      bool
	rtpMaxPayloadSize    int
	replaceNTP           bool
	inboundFramesInError *errordumper.Dumper
	inboundBytes         *atomic.Uint64
	outboundBytes        *atomic.Uint64
	updateLastTime       func(time.Duration)
	writeRTSP            func([]*rtp.Packet, time.Time)
	updateOutDesc        func(func())
	parent               logger.Writer

	outFormat     format.Format
	forceRemux    bool
	ptsOffset     int64
	formatUpdater formatUpdater
	unitRemuxer   unitRemuxer
	rtpEncoder    rtpEncoder
	rtpTimeOffset uint32
	ntpEstimator  *ntpestimator.Estimator
	onDatas       map[*Reader]OnDataFunc
}

func (sf *streamFormat) initialize() error {
	sf.outFormat = cloneFormatShallow(sf.origFormat)

	if forma, ok := sf.outFormat.(*format.H264); ok && forma.PacketizationMode == 0 {
		sf.parent.Log(logger.Info, "remuxing in order to change H264 packetization-mode from 0 to 1")
		forma.PacketizationMode = 1
		sf.forceRemux = true
	}

	sf.formatUpdater = newFormatUpdater(sf.outFormat)
	sf.unitRemuxer = newUnitRemuxer(sf.outFormat)

	if sf.replaceNTP {
		sf.ntpEstimator = &ntpestimator.Estimator{
			ClockRate: sf.outFormat.ClockRate(),
		}
	}

	sf.onDatas = make(map[*Reader]OnDataFunc)

	return nil
}
