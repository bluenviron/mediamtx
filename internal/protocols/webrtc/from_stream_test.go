package webrtc

import (
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.MJPEG{}},
	}}}

	r := &stream.Reader{
		Parent: test.Logger(func(logger.Level, string, ...interface{}) {
			t.Error("should not happen")
		}),
	}

	err := FromStream(desc, r, nil)
	require.Equal(t, errNoSupportedCodecsFrom, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.MJPEG{}},
		},
	}}

	n := 0

	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...interface{}) {
			require.Equal(t, logger.Warn, l)
			if n == 0 {
				require.Equal(t, "skipping track 2 (M-JPEG)", fmt.Sprintf(format, args...))
			}
			n++
		}),
	}

	pc := &PeerConnection{}

	err := FromStream(desc, r, pc)
	require.NoError(t, err)

	require.Equal(t, 1, n)
}

func TestFromStream(t *testing.T) {
	for _, ca := range toFromStreamCases {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{
				Medias: []*description.Media{{
					Formats: []format.Format{ca.in},
				}},
			}

			pc := &PeerConnection{}
			r := &stream.Reader{Parent: test.NilLogger}

			err := FromStream(desc, r, pc)
			require.NoError(t, err)

			require.Equal(t, ca.webrtcCaps, pc.OutgoingTracks[0].Caps)
		})
	}
}

func TestFromStreamResampleOpus(t *testing.T) {
	strm := &stream.Stream{
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Desc: &description.Session{Medias: []*description.Media{
			{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					ChannelCount: 2,
				}},
			},
		}},
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	pc1 := &PeerConnection{
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            false,
		Log:                test.NilLogger,
	}
	err = pc1.Start()
	require.NoError(t, err)
	defer pc1.Close()

	pc2 := &PeerConnection{
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            true,
		Log:                test.NilLogger,
	}

	r := &stream.Reader{Parent: nil}

	err = FromStream(strm.Desc, r, pc2)
	require.NoError(t, err)

	err = pc2.Start()
	require.NoError(t, err)
	defer pc2.Close()

	offer, err := pc1.CreatePartialOffer()
	require.NoError(t, err)

	answer, err := pc2.CreateFullAnswer(offer)
	require.NoError(t, err)

	err = pc1.SetAnswer(answer)
	require.NoError(t, err)

	err = pc1.WaitUntilConnected()
	require.NoError(t, err)

	err = pc2.WaitUntilConnected()
	require.NoError(t, err)

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	strm.WriteRTPPacket(strm.Desc.Medias[0], strm.Desc.Medias[0].Formats[0], &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    111,
			SequenceNumber: 1123,
			Timestamp:      45343,
			SSRC:           563424,
		},
		Payload: []byte{1},
	}, time.Now(), 0)

	strm.WriteRTPPacket(strm.Desc.Medias[0], strm.Desc.Medias[0].Formats[0], &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    111,
			SequenceNumber: 1124,
			Timestamp:      45343,
			SSRC:           563424,
		},
		Payload: []byte{1},
	}, time.Now(), 0)

	err = pc1.GatherIncomingTracks()
	require.NoError(t, err)

	tracks := pc1.IncomingTracks()

	done := make(chan struct{})
	n := 0
	var ts uint32

	tracks[0].OnPacketRTP = func(pkt *rtp.Packet, _ time.Time) {
		n++

		switch n {
		case 1:
			ts = pkt.Timestamp

		case 2:
			require.Equal(t, uint32(960), pkt.Timestamp-ts)
			close(done)
		}
	}

	pc1.StartReading()

	<-done
}
