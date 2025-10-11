package webrtc

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/rtpreceiver"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	keyFrameInterval  = 2 * time.Second
	mimeTypeMultiopus = "audio/multiopus"
	mimeTypeL16       = "audio/L16"
)

var incomingVideoCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeAV1,
			ClockRate:   90000,
			SDPFmtpLine: "profile=1",
		},
		PayloadType: 96,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeAV1,
			ClockRate: 90000,
		},
		PayloadType: 97,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=3",
		},
		PayloadType: 98,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=2",
		},
		PayloadType: 99,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=1",
		},
		PayloadType: 100,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=0",
		},
		PayloadType: 101,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 102,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH265,
			ClockRate:   90000,
			SDPFmtpLine: "level-id=93;profile-id=2;tier-flag=0;tx-mode=SRST",
		},
		PayloadType: 103,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH265,
			ClockRate:   90000,
			SDPFmtpLine: "level-id=93;profile-id=1;tier-flag=0;tx-mode=SRST",
		},
		PayloadType: 104,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 105,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 106,
	},
}

var incomingAudioCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    3,
			SDPFmtpLine: "channel_mapping=0,2,1;num_streams=2;coupled_streams=1",
		},
		PayloadType: 112,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    4,
			SDPFmtpLine: "channel_mapping=0,1,2,3;num_streams=2;coupled_streams=2",
		},
		PayloadType: 113,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    5,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3;num_streams=3;coupled_streams=2",
		},
		PayloadType: 114,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    6,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3,5;num_streams=4;coupled_streams=2",
		},
		PayloadType: 115,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    7,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3,5,6;num_streams=4;coupled_streams=4",
		},
		PayloadType: 116,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    8,
			SDPFmtpLine: "channel_mapping=0,6,1,4,5,2,3,7;num_streams=5;coupled_streams=4",
		},
		PayloadType: 117,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
		},
		PayloadType: 111,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeG722,
			ClockRate: 8000,
		},
		PayloadType: 9,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  2,
		},
		PayloadType: 118,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
			Channels:  2,
		},
		PayloadType: 119,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
		},
		PayloadType: 0,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
		},
		PayloadType: 8,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  mimeTypeL16,
			ClockRate: 8000,
			Channels:  2,
		},
		PayloadType: 120,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  mimeTypeL16,
			ClockRate: 16000,
			Channels:  2,
		},
		PayloadType: 121,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  mimeTypeL16,
			ClockRate: 48000,
			Channels:  2,
		},
		PayloadType: 122,
	},
}

// IncomingTrack is an incoming track.
type IncomingTrack struct {
	OnPacketRTP func(*rtp.Packet)

	track              *webrtc.TrackRemote
	receiver           *webrtc.RTPReceiver
	writeRTCP          func([]rtcp.Packet) error
	log                logger.Writer
	rtpPacketsReceived *uint64
	rtpPacketsLost     *uint64

	packetsLost  *counterdumper.CounterDumper
	rtcpReceiver *rtpreceiver.Receiver
}

func (t *IncomingTrack) initialize() {
	t.OnPacketRTP = func(*rtp.Packet) {}
}

// Codec returns the track codec.
func (t *IncomingTrack) Codec() webrtc.RTPCodecParameters {
	return t.track.Codec()
}

// ClockRate returns the clock rate. Needed by rtptime.GlobalDecoder
func (t *IncomingTrack) ClockRate() int {
	return int(t.track.Codec().ClockRate)
}

// PTSEqualsDTS returns whether PTS equals DTS. Needed by rtptime.GlobalDecoder
func (*IncomingTrack) PTSEqualsDTS(*rtp.Packet) bool {
	return true
}

func (t *IncomingTrack) start() {
	t.packetsLost = &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			t.log.Log(logger.Warn, "%d RTP %s lost",
				val,
				func() string {
					if val == 1 {
						return "packet"
					}
					return "packets"
				}())
		},
	}
	t.packetsLost.Start()

	t.rtcpReceiver = &rtpreceiver.Receiver{
		ClockRate:            int(t.track.Codec().ClockRate),
		UnrealiableTransport: true,
		Period:               1 * time.Second,
		WritePacketRTCP: func(p rtcp.Packet) {
			t.writeRTCP([]rtcp.Packet{p}) //nolint:errcheck
		},
	}
	err := t.rtcpReceiver.Initialize()
	if err != nil {
		panic(err)
	}

	// read incoming RTCP packets.
	// incoming RTCP packets must always be read to make interceptors work.
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err2 := t.receiver.Read(buf)
			if err2 != nil {
				return
			}

			pkts, err2 := rtcp.Unmarshal(buf[:n])
			if err2 != nil {
				panic(err2)
			}

			for _, pkt := range pkts {
				if sr, ok := pkt.(*rtcp.SenderReport); ok {
					t.rtcpReceiver.ProcessSenderReport(sr, time.Now())
				}
			}
		}
	}()

	// send period key frame requests
	if t.track.Kind() == webrtc.RTPCodecTypeVideo {
		go func() {
			keyframeTicker := time.NewTicker(keyFrameInterval)
			defer keyframeTicker.Stop()

			for range keyframeTicker.C {
				err2 := t.writeRTCP([]rtcp.Packet{
					&rtcp.PictureLossIndication{
						MediaSSRC: uint32(t.track.SSRC()),
					},
				})
				if err2 != nil {
					return
				}
			}
		}()
	}

	// read incoming RTP packets.
	go func() {
		for {
			pkt, _, err2 := t.track.ReadRTP()
			if err2 != nil {
				return
			}

			packets, lost, err2 := t.rtcpReceiver.ProcessPacket(pkt, time.Now(), true)
			if err2 != nil {
				t.log.Log(logger.Warn, err2.Error())
				continue
			}
			if lost != 0 {
				atomic.AddUint64(t.rtpPacketsLost, lost)
				t.packetsLost.Add(lost)
				// do not return
			}

			atomic.AddUint64(t.rtpPacketsReceived, uint64(len(packets)))

			for _, pkt := range packets {
				// sometimes Chrome sends empty RTP packets. ignore them.
				if len(pkt.Payload) == 0 {
					continue
				}

				t.OnPacketRTP(pkt)
			}
		}
	}()
}

// PacketNTP returns the packet NTP.
func (t *IncomingTrack) PacketNTP(pkt *rtp.Packet) (time.Time, bool) {
	return t.rtcpReceiver.PacketNTP(pkt.Timestamp)
}

func (t *IncomingTrack) close() {
	if t.packetsLost != nil {
		t.packetsLost.Stop()
	}
	if t.rtcpReceiver != nil {
		t.rtcpReceiver.Close()
	}
}
