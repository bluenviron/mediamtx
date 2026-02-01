// Package rtsp provides RTSP utilities.
package rtsp

import (
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/pion/rtp"
)

type ntpState int

const (
	ntpStateInitial ntpState = iota
	ntpStateReplace
	ntpStateAvailable
)

type rtspSource interface {
	PacketPTS(*description.Media, *rtp.Packet) (int64, bool)
	PacketNTP(*description.Media, *rtp.Packet) (time.Time, bool)
	OnPacketRTP(*description.Media, format.Format, gortsplib.OnPacketRTPFunc)
}

// ToStream maps a RTSP stream to a MediaMTX stream.
func ToStream(
	source rtspSource,
	medias []*description.Media,
	pathConf *conf.Path,
	strm *stream.Stream,
	log logger.Writer,
) {
	for _, medi := range medias {
		for _, forma := range medi.Formats {
			cmedi := medi
			cforma := forma

			var ntpStat ntpState

			// When frame metadata is enabled, prefer preserving camera-provided absolute time
			// (when available) even if the user disabled useAbsoluteTimestamp, since metadata
			// explicitly carries both camera and ingest timelines.
			if !pathConf.UseAbsoluteTimestamp && !pathConf.EnableFrameMetadata {
				ntpStat = ntpStateReplace
			}

			handleNTP := func(pkt *rtp.Packet) (time.Time, bool) {
				switch ntpStat {
				case ntpStateReplace:
					return time.Time{}, true

				case ntpStateInitial:
					ntp, avail := source.PacketNTP(cmedi, pkt)
					if !avail {
						// If frame metadata is enabled, we can still proceed without camera NTP:
						// metadata will fall back to stream PTS (and/or an estimator) when NTP is missing.
						if pathConf.EnableFrameMetadata {
							return time.Time{}, true
						}

						log.Log(logger.Warn, "received RTP packet without absolute time, skipping it")
						return time.Time{}, false
					}

					ntpStat = ntpStateAvailable
					return ntp, true

				default: // ntpStateAvailable
					ntp, avail := source.PacketNTP(cmedi, pkt)
					if !avail {
						panic("should not happen")
					}

					return ntp, true
				}
			}

			source.OnPacketRTP(cmedi, cforma, func(pkt *rtp.Packet) {
				pts, ok := source.PacketPTS(cmedi, pkt)
				if !ok {
					return
				}

				ntp, ok := handleNTP(pkt)
				if !ok {
					return
				}

				strm.WriteRTPPacket(cmedi, cforma, pkt, ntp, pts)
			})
		}
	}
}
