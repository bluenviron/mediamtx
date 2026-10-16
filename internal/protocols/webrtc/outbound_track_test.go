package webrtc

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/rtpsender"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestOutboundTrackReceptionReports(t *testing.T) {
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

	sent, reportedLost, reportedJitter := outboundTracksStats(tracks)
	require.Equal(t, uint64(2), sent)
	require.Equal(t, uint64(0), reportedLost)
	require.Equal(t, float64(0), reportedJitter)

	tr1.processRTCP([]rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{
				// reports of other SSRCs must be ignored
				{SSRC: 9999, TotalLost: 1000, Jitter: 1000},
				{SSRC: 1234, TotalLost: 10, Jitter: 300},
			},
		},
		&rtcp.PictureLossIndication{MediaSSRC: 1234},
	})

	sent, reportedLost, reportedJitter = outboundTracksStats(tracks)
	require.Equal(t, uint64(2), sent)
	require.Equal(t, uint64(10), reportedLost)
	require.Equal(t, float64(300), reportedJitter)

	// reception reports are inside sender reports when the remote peer is sending too
	tr2.processRTCP([]rtcp.Packet{
		&rtcp.SenderReport{
			Reports: []rtcp.ReceptionReport{
				{SSRC: 5678, TotalLost: 5, Jitter: 100},
			},
		},
	})

	sent, reportedLost, reportedJitter = outboundTracksStats(tracks)
	require.Equal(t, uint64(2), sent)
	require.Equal(t, uint64(15), reportedLost)
	require.Equal(t, float64(200), reportedJitter)
}
