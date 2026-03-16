package stream

import (
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/pion/rtp"
)

type streamMedia struct {
	media                *description.Media
	alwaysAvailable      bool
	rtpMaxPayloadSize    int
	replaceNTP           bool
	addInboundBytes      func(uint64)
	addOutboundBytes     func(uint64)
	updateLastTime       func(time.Duration)
	writeRTSP            func(*description.Media, []*rtp.Packet, time.Time)
	inboundFramesInError *errordumper.Dumper
	parent               logger.Writer

	formats map[format.Format]*streamFormat
}

func (sm *streamMedia) initialize() error {
	sm.formats = make(map[format.Format]*streamFormat)

	for _, forma := range sm.media.Formats {
		sf := &streamFormat{
			format:               forma,
			media:                sm.media,
			alwaysAvailable:      sm.alwaysAvailable,
			rtpMaxPayloadSize:    sm.rtpMaxPayloadSize,
			replaceNTP:           sm.replaceNTP,
			inboundFramesInError: sm.inboundFramesInError,
			addInboundBytes:      sm.addInboundBytes,
			addOutboundBytes:     sm.addOutboundBytes,
			updateLastTime:       sm.updateLastTime,
			writeRTSP:            sm.writeRTSP,
			parent:               sm.parent,
		}
		err := sf.initialize()
		if err != nil {
			return err
		}
		sm.formats[forma] = sf
	}

	return nil
}
