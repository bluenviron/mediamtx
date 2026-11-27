package stream

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamMedia struct {
	origMedia                       *description.Media
	alwaysAvailable                 bool
	rtspRTPMaxUnencryptedPacketSize int
	rtspRTPMaxPayloadSize           int
	replaceNTP                      bool
	inboundBytes                    *atomic.Uint64
	outboundBytes                   *atomic.Uint64
	writeRTSP                       func(*description.Media, []*rtp.Packet, time.Time)
	updateOutDesc                   func(func())
	inboundFramesInError            *errordumper.Dumper
	parent                          logger.Writer

	outMedia *description.Media
	formats  map[format.Format]*streamFormat
}

func (sm *streamMedia) initialize() error {
	sm.formats = make(map[format.Format]*streamFormat)

	sm.outMedia = &description.Media{
		Type:     sm.origMedia.Type,
		ID:       sm.origMedia.ID,
		Language: sm.origMedia.Language,
		Formats:  make([]format.Format, len(sm.origMedia.Formats)),
	}

	// advertise the NTP-64 header extension (RFC 6051) only when absolute timestamps are kept,
	// since it would contradict the replaced NTP of outgoing sender reports.
	if !sm.replaceNTP {
		for _, ext := range sm.origMedia.HeaderExtensions {
			if ext.URI == description.NTP64ExtensionURI {
				sm.outMedia.HeaderExtensions = []description.HeaderExtension{ext}
				break
			}
		}
	}

	for i, origFormat := range sm.origMedia.Formats {
		sf := &streamFormat{
			origFormat:                      origFormat,
			alwaysAvailable:                 sm.alwaysAvailable,
			rtspRTPMaxUnencryptedPacketSize: sm.rtspRTPMaxUnencryptedPacketSize,
			rtspRTPMaxPayloadSize:           sm.rtspRTPMaxPayloadSize,
			replaceNTP:                      sm.replaceNTP,
			inboundFramesInError:            sm.inboundFramesInError,
			inboundBytes:                    sm.inboundBytes,
			outboundBytes:                   sm.outboundBytes,
			writeRTSP:                       sm.writeRTSPWrapper,
			updateOutDesc:                   sm.updateOutDesc,
			parent:                          sm.parent,
		}
		err := sf.initialize()
		if err != nil {
			return err
		}

		sm.formats[origFormat] = sf
		sm.outMedia.Formats[i] = sf.outFormat
	}

	return nil
}

func (sm *streamMedia) writeRTSPWrapper(pkts []*rtp.Packet, ntp time.Time) {
	sm.writeRTSP(sm.outMedia, pkts, ntp)
}
