package formatprocessor

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type testLogger struct {
	cb func(level logger.Level, format string, args ...interface{})
}

func (l *testLogger) Log(level logger.Level, format string, args ...interface{}) {
	l.cb(level, format, args...)
}

// Logger returns a dummy logger.
func Logger(cb func(logger.Level, string, ...interface{})) logger.Writer {
	return &testLogger{cb: cb}
}

func TestH264ProcessUnit(t *testing.T) {
	forma := &format.H264{}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	u1 := &unit.H264{
		Base: unit.Base{
			PTS: 30000,
		},
		AU: [][]byte{
			{7, 4, 5, 6}, // SPS
			{8, 1},       // PPS
			{5, 1},       // IDR
		},
	}

	err = p.ProcessUnit(u1)
	require.NoError(t, err)

	require.Equal(t, [][]byte{
		{7, 4, 5, 6}, // SPS
		{8, 1},       // PPS
		{5, 1},       // IDR
	}, u1.AU)

	u2 := &unit.H264{
		Base: unit.Base{
			PTS: 30000 * 2,
		},
		AU: [][]byte{
			{5, 2}, // IDR
		},
	}

	err = p.ProcessUnit(u2)
	require.NoError(t, err)

	// test that params have been added to the SDP
	require.Equal(t, []byte{7, 4, 5, 6}, forma.SPS)
	require.Equal(t, []byte{8, 1}, forma.PPS)

	// test that params have been added to the frame
	require.Equal(t, [][]byte{
		{7, 4, 5, 6}, // SPS
		{8, 1},       // PPS
		{5, 2},       // IDR
	}, u2.AU)

	// test that timestamp had increased
	require.Equal(t, u1.RTPPackets[0].Timestamp+30000, u2.RTPPackets[0].Timestamp)
}

func TestH264ProcessUnitEmpty(t *testing.T) {
	forma := &format.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	unit := &unit.H264{
		AU: [][]byte{
			{0x07, 0x01, 0x02, 0x03}, // SPS
			{0x08, 0x01, 0x02},       // PPS
		},
	}

	err = p.ProcessUnit(unit)
	require.NoError(t, err)

	// if all NALUs have been removed, no RTP packets shall be generated.
	require.Equal(t, []*rtp.Packet(nil), unit.RTPPackets)
}

func TestH264ProcessRTPPacketUpdateParams(t *testing.T) {
	for _, ca := range []string{"standard", "aggregated"} {
		t.Run(ca, func(t *testing.T) {
			forma := &format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
			}

			p, err := New(1450, forma, false, nil)
			require.NoError(t, err)

			enc, err := forma.CreateEncoder()
			require.NoError(t, err)

			pkts, err := enc.Encode([][]byte{{byte(mch264.NALUTypeIDR)}})
			require.NoError(t, err)

			data, err := p.ProcessRTPPacket(pkts[0], time.Time{}, 0, true)
			require.NoError(t, err)

			require.Equal(t, [][]byte{
				{byte(mch264.NALUTypeIDR)},
			}, data.(*unit.H264).AU)

			if ca == "standard" {
				pkts, err = enc.Encode([][]byte{{7, 4, 5, 6}}) // SPS
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)

				pkts, err = enc.Encode([][]byte{{8, 1}}) // PPS
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)
			} else {
				pkts, err = enc.Encode([][]byte{
					{7, 4, 5, 6}, // SPS
					{8, 1},       // PPS
				})
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)
			}

			require.Equal(t, []byte{7, 4, 5, 6}, forma.SPS)
			require.Equal(t, []byte{8, 1}, forma.PPS)

			pkts, err = enc.Encode([][]byte{{byte(mch264.NALUTypeIDR)}})
			require.NoError(t, err)

			data, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, true)
			require.NoError(t, err)

			require.Equal(t, [][]byte{
				{0x07, 4, 5, 6},
				{0x08, 1},
				{byte(mch264.NALUTypeIDR)},
			}, data.(*unit.H264).AU)
		})
	}
}

func TestH264ProcessRTPPacketOversized(t *testing.T) {
	forma := &format.H264{
		PayloadTyp:        96,
		SPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PacketizationMode: 1,
	}

	logged := false

	p, err := New(1460, forma, false,
		Logger(func(_ logger.Level, s string, i ...interface{}) {
			require.Equal(t, "RTP packets are too big, remuxing them into smaller ones", fmt.Sprintf(s, i...))
			logged = true
		}))
	require.NoError(t, err)

	var out []*rtp.Packet

	for _, pkt := range []*rtp.Packet{
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         false,
				PayloadType:    96,
				SequenceNumber: 124,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: append([]byte{0x1c, 0b10000000}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4)...),
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 125,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: []byte{0x1c, 0b01000000, 0x01, 0x02, 0x03, 0x04},
		},
	} {
		data, err := p.ProcessRTPPacket(pkt, time.Time{}, 0, false)
		require.NoError(t, err)

		out = append(out, data.GetRTPPackets()...)
	}

	require.Equal(t, []*rtp.Packet{
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         false,
				PayloadType:    96,
				SequenceNumber: 124,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: append(
				append([]byte{0x1c, 0x80}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 364)...),
				[]byte{0x01, 0x02}...,
			),
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 125,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: append(
				[]byte{0x1c, 0x40, 0x03, 0x04},
				bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 136)...,
			),
		},
	}, out)

	require.True(t, logged)
}

func FuzzRTPH264ExtractParams(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		rtpH264ExtractParams(b)
	})
}
