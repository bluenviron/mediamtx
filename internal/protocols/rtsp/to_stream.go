// Package rtsp provides RTSP utilities.
package rtsp

import (
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
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
	PacketPTS2(*description.Media, *rtp.Packet) (int64, bool)
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

			if !pathConf.UseAbsoluteTimestamp {
				ntpStat = ntpStateReplace
			}

			handleNTP := func(pkt *rtp.Packet) (time.Time, bool) {
				switch ntpStat {
				case ntpStateReplace:
					return time.Now(), true

				case ntpStateInitial:
					ntp, avail := source.PacketNTP(cmedi, pkt)
					if !avail {
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
				pts, ok := source.PacketPTS2(cmedi, pkt)
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
