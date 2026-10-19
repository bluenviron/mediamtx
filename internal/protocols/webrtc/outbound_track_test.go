package webrtc

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/rtpsender"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestOutboundTrackRTCPFeedback(t *testing.T) {
	newTrack := func(ssrc uint32) *OutboundTrack {
		tr := &OutboundTrack{
			ssrc: ssrc,
			rtcpSender: &rtpsender.Sender{
				ClockRate:       90000,
				Period:          1 * time.Hour,
				TimeNow:         time.Now,
				WritePacketRTCP: func(rtcp.Packet) {},
			},
		}
		tr.rtcpSender.Initialize()

		tr.rtcpSender.ProcessPacket(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				SequenceNumber: 1,
				SSRC:           ssrc,
			},
		}, time.Now(), true)

		return tr
	}

	tr1 := newTrack(1234)
	defer tr1.close()

	tr2 := newTrack(5678)
	defer tr2.close()

	tracks := []*OutboundTrack{tr1, tr2}

	require.Equal(t, outboundTracksStatistics{
		packetsSent: 2,
	}, outboundTracksStats(tracks))

	// a compound packet is delivered to every track whose SSRC it mentions.
	compound := []rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{
				{SSRC: 1234, TotalLost: 10, Jitter: 300},
				// reports of other SSRCs must be ignored
				{SSRC: 9999, TotalLost: 1000, Jitter: 1000},
			},
		},
		&rtcp.TransportLayerNack{
			MediaSSRC: 1234,
			Nacks:     []rtcp.NackPair{{PacketID: 10, LostPackets: 0b101}},
		},
		&rtcp.TransportLayerNack{MediaSSRC: 1234},
		&rtcp.PictureLossIndication{MediaSSRC: 1234},
		&rtcp.TransportLayerNack{MediaSSRC: 9999},
		&rtcp.PictureLossIndication{MediaSSRC: 9999},
	}
	tr1.processRTCP(compound)
	tr2.processRTCP(compound)

	require.Equal(t, outboundTracksStatistics{
		packetsSent:         2,
		packetsReportedLost: 10,
		reportedJitter:      300,
		nackPackets:         2,
		pliPackets:          1,
	}, outboundTracksStats(tracks))

	// reception reports are inside sender reports when the remote peer is sending too
	tr2.processRTCP([]rtcp.Packet{
		&rtcp.SenderReport{
			Reports: []rtcp.ReceptionReport{
				{SSRC: 5678, TotalLost: 5, Jitter: 100},
			},
		},
		&rtcp.PictureLossIndication{MediaSSRC: 5678},
	})

	require.Equal(t, outboundTracksStatistics{
		packetsSent:         2,
		packetsReportedLost: 15,
		reportedJitter:      200,
		nackPackets:         2,
		pliPackets:          2,
	}, outboundTracksStats(tracks))
}
