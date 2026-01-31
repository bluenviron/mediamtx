package stream

import (
	"crypto/rand"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
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
	format            format.Format
	media             *description.Media
	alwaysAvailable   bool
	rtpMaxPayloadSize int
	replaceNTP        bool
	processingErrors  *errordumper.Dumper
	onBytesReceived   func(uint64)
	onBytesSent       func(uint64)
	writeRTSP         func(*description.Media, []*rtp.Packet, time.Time)
	parent            logger.Writer

	firstReceived  bool
	lastPTS        int64
	lastSystemTime time.Time
	ptsOffset      int64
	formatUpdater  formatUpdater
	unitRemuxer    unitRemuxer
	rtpEncoder     rtpEncoder
	rtpTimeOffset  uint32
	ntpEstimator   *ntpestimator.Estimator
	onDatas        map[*Reader]OnDataFunc
}

func (sf *streamFormat) initialize() error {
	sf.lastSystemTime = time.Now()

	sf.formatUpdater = newFormatUpdater(sf.format)
	sf.unitRemuxer = newUnitRemuxer(sf.format)

	if sf.replaceNTP {
		sf.ntpEstimator = &ntpestimator.Estimator{
			ClockRate: sf.format.ClockRate(),
		}
	}

	sf.onDatas = make(map[*Reader]OnDataFunc)

	return nil
}
