package formatprocessor

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	mch265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestH265ProcessUnit(t *testing.T) {
	forma := &format.H265{}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	u1 := &unit.H265{
		Base: unit.Base{
			PTS: 30000,
		},
		AU: [][]byte{
			{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3},
			{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6},
			{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9},
			{byte(mch265.NALUType_CRA_NUT) << 1, 0},
		},
	}

	err = p.ProcessUnit(u1)
	require.NoError(t, err)

	require.Equal(t, [][]byte{
		{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3},
		{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6},
		{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9},
		{byte(mch265.NALUType_CRA_NUT) << 1, 0},
	}, u1.AU)

	u2 := &unit.H265{
		Base: unit.Base{
			PTS: 30000 * 2,
		},
		AU: [][]byte{
			{byte(mch265.NALUType_CRA_NUT) << 1, 1},
		},
	}

	err = p.ProcessUnit(u2)
	require.NoError(t, err)

	// test that params have been added to the SDP
	require.Equal(t, []byte{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3}, forma.VPS)
	require.Equal(t, []byte{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6}, forma.SPS)
	require.Equal(t, []byte{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9}, forma.PPS)

	// test that params have been added to the frame
	require.Equal(t, [][]byte{
		{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3},
		{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6},
		{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9},
		{byte(mch265.NALUType_CRA_NUT) << 1, 1},
	}, u2.AU)

	// test that timestamp had increased
	require.Equal(t, u1.RTPPackets[0].Timestamp+30000, u2.RTPPackets[0].Timestamp)
}

func TestH265ProcessUnitEmpty(t *testing.T) {
	forma := &format.H265{
		PayloadTyp: 96,
	}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	unit := &unit.H265{
		AU: [][]byte{
			{byte(mch265.NALUType_VPS_NUT) << 1, 10, 11, 12}, // VPS
			{byte(mch265.NALUType_SPS_NUT) << 1, 13, 14, 15}, // SPS
			{byte(mch265.NALUType_PPS_NUT) << 1, 16, 17, 18}, // PPS
		},
	}

	err = p.ProcessUnit(unit)
	require.NoError(t, err)

	// if all NALUs have been removed, no RTP packets shall be generated.
	require.Equal(t, []*rtp.Packet(nil), unit.RTPPackets)
}

func TestH265ProcessRTPPacketUpdateParams(t *testing.T) {
	for _, ca := range []string{"standard", "aggregated"} {
		t.Run(ca, func(t *testing.T) {
			forma := &format.H265{
				PayloadTyp: 96,
			}

			p, err := New(1450, forma, false, nil)
			require.NoError(t, err)

			enc, err := forma.CreateEncoder()
			require.NoError(t, err)

			pkts, err := enc.Encode([][]byte{{byte(mch265.NALUType_CRA_NUT) << 1, 0}})
			require.NoError(t, err)

			data, err := p.ProcessRTPPacket(pkts[0], time.Time{}, 0, true)
			require.NoError(t, err)

			require.Equal(t, [][]byte{
				{byte(mch265.NALUType_CRA_NUT) << 1, 0},
			}, data.(*unit.H265).AU)

			if ca == "standard" {
				pkts, err = enc.Encode([][]byte{{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3}})
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)

				pkts, err = enc.Encode([][]byte{{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6}})
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)

				pkts, err = enc.Encode([][]byte{{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9}})
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)
			} else {
				pkts, err = enc.Encode([][]byte{
					{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3},
					{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6},
					{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9},
				})
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)
			}

			require.Equal(t, []byte{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3}, forma.VPS)
			require.Equal(t, []byte{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6}, forma.SPS)
			require.Equal(t, []byte{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9}, forma.PPS)

			pkts, err = enc.Encode([][]byte{{byte(mch265.NALUType_CRA_NUT) << 1, 0}})
			require.NoError(t, err)

			data, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, true)
			require.NoError(t, err)

			require.Equal(t, [][]byte{
				{byte(mch265.NALUType_VPS_NUT) << 1, 1, 2, 3},
				{byte(mch265.NALUType_SPS_NUT) << 1, 4, 5, 6},
				{byte(mch265.NALUType_PPS_NUT) << 1, 7, 8, 9},
				{byte(mch265.NALUType_CRA_NUT) << 1, 0},
			}, data.(*unit.H265).AU)
		})
	}
}

func TestH265ProcessRTPPacketOversized(t *testing.T) {
	forma := &format.H265{
		PayloadTyp: 96,
		VPS:        []byte{byte(mch265.NALUType_VPS_NUT) << 1, 10, 11, 12},
		SPS:        []byte{byte(mch265.NALUType_SPS_NUT) << 1, 13, 14, 15},
		PPS:        []byte{byte(mch265.NALUType_PPS_NUT) << 1, 16, 17, 18},
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
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 124,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4),
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
				append([]byte{0x63, 0x02, 0x80, 0x03, 0x04}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 363)...),
				[]byte{0x01, 0x02, 0x03}...,
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
				[]byte{0x63, 0x02, 0x40, 0x04},
				bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 135)...,
			),
		},
	}, out)

	require.True(t, logged)
}

func FuzzRTPH265ExtractParams(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		rtpH265ExtractParams(b)
	})
}
