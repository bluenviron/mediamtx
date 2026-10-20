package webrtc

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/rtpsender"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// OutboundTrack is an outgoing track.
type OutboundTrack struct {
	Caps webrtc.RTPCodecCapability

	track          *webrtc.TrackLocalStaticRTP
	ssrc           uint32
	rtcpSender     *rtpsender.Sender
	reportReceived atomic.Bool
	reportedJitter atomic.Uint32
	nackPackets    atomic.Uint64
	pliPackets     atomic.Uint64
}

func (t *OutboundTrack) isVideo() bool {
	return strings.Split(t.Caps.MimeType, "/")[0] == "video"
}

func (t *OutboundTrack) setup(p *PeerConnection) error {
	var trackID string
	if t.isVideo() {
		trackID = "video"
	} else {
		trackID = "audio"
	}

	var err error
	t.track, err = webrtc.NewTrackLocalStaticRTP(
		t.Caps,
		trackID,
		webrtcStreamID,
	)
	if err != nil {
		return err
	}

	sender, err := p.wr.AddTrack(t.track)
	if err != nil {
		return err
	}

	t.ssrc = uint32(sender.GetParameters().Encodings[0].SSRC)

	t.rtcpSender = &rtpsender.Sender{
		ClockRate: int(t.track.Codec().ClockRate),
		Period:    1 * time.Second,
		TimeNow:   time.Now,
		WritePacketRTCP: func(pkt rtcp.Packet) {
			p.wr.WriteRTCP([]rtcp.Packet{pkt}) //nolint:errcheck
		},
	}
	t.rtcpSender.Initialize()

	// incoming RTCP packets must always be read to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err2 := sender.Read(buf)
			if err2 != nil {
				return
			}

			pkts, err2 := rtcp.Unmarshal(buf[:n])
			if err2 != nil {
				panic(err2)
			}

			t.processRTCP(pkts)
		}
	}()

	return nil
}

func (t *OutboundTrack) processRTCP(pkts []rtcp.Packet) {
	for _, pkt := range pkts {
		switch pkt := pkt.(type) {
		case *rtcp.ReceiverReport:
			t.processReceptionReports(pkt.Reports)

		// reception reports are inside sender reports when the remote peer is sending too
		case *rtcp.SenderReport:
			t.processReceptionReports(pkt.Reports)

		// a compound packet is delivered to every track whose SSRC it mentions,
		// therefore feedback must be filtered by SSRC to be counted once.
		case *rtcp.TransportLayerNack:
			if pkt.MediaSSRC == t.ssrc {
				t.nackPackets.Add(1)
			}

		case *rtcp.PictureLossIndication:
			if pkt.MediaSSRC == t.ssrc {
				t.pliPackets.Add(1)
			}
		}
	}
}

func (t *OutboundTrack) processReceptionReports(reports []rtcp.ReceptionReport) {
	for _, report := range reports {
		if report.SSRC == t.ssrc {
			t.rtcpSender.ProcessReceptionReport(&report)
			t.reportedJitter.Store(report.Jitter)
			t.reportReceived.Store(true)
		}
	}
}

type outboundTracksStatistics struct {
	packetsSent         uint64
	packetsReportedLost uint64
	reportedJitter      float64
	nackPackets         uint64
	pliPackets          uint64
}

func outboundTracksStats(tracks []*OutboundTrack) outboundTracksStatistics {
	var st outboundTracksStatistics
	n := float64(0)

	for _, tr := range tracks {
		if sentStats := tr.rtcpSender.Stats(); sentStats != nil {
			st.packetsSent += sentStats.Sent
			st.packetsReportedLost += sentStats.ReportedLost
		}

		if tr.reportReceived.Load() {
			st.reportedJitter += float64(tr.reportedJitter.Load())
			n++
		}

		st.nackPackets += tr.nackPackets.Load()
		st.pliPackets += tr.pliPackets.Load()
	}

	if n != 0 {
		st.reportedJitter /= n
	}

	return st
}

func (t *OutboundTrack) close() {
	if t.rtcpSender != nil {
		t.rtcpSender.Close()
	}
}

// WriteRTP writes a RTP packet.
func (t *OutboundTrack) WriteRTP(pkt *rtp.Packet) error {
	return t.WriteRTPWithNTP(pkt, time.Now())
}

// WriteRTPWithNTP writes a RTP packet.
func (t *OutboundTrack) WriteRTPWithNTP(pkt *rtp.Packet, ntp time.Time) error {
	// use right SSRC in packet to make rtcpSender work
	pkt.SSRC = t.ssrc

	t.rtcpSender.ProcessPacket(pkt, ntp, true)

	return t.track.WriteRTP(pkt)
}
