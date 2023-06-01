package formatprocessor

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type testLogWriter struct {
	recv chan string
}

func (w *testLogWriter) Log(_ logger.Level, format string, args ...interface{}) {
	w.recv <- fmt.Sprintf(format, args...)
}

func TestH264DynamicParams(t *testing.T) {
	forma := &formats.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	p, err := New(1472, forma, false, nil)
	require.NoError(t, err)

	enc, err := forma.CreateEncoder2()
	require.NoError(t, err)

	pkts, err := enc.Encode([][]byte{{byte(h264.NALUTypeIDR)}}, 0)
	require.NoError(t, err)

	data := &UnitH264{RTPPackets: []*rtp.Packet{pkts[0]}}
	p.Process(data, true)

	require.Equal(t, [][]byte{
		{byte(h264.NALUTypeIDR)},
	}, data.AU)

	pkts, err = enc.Encode([][]byte{{7, 4, 5, 6}}, 0) // SPS
	require.NoError(t, err)
	p.Process(&UnitH264{RTPPackets: []*rtp.Packet{pkts[0]}}, false)

	pkts, err = enc.Encode([][]byte{{8, 1}}, 0) // PPS
	require.NoError(t, err)
	p.Process(&UnitH264{RTPPackets: []*rtp.Packet{pkts[0]}}, false)

	require.Equal(t, []byte{7, 4, 5, 6}, forma.SPS)
	require.Equal(t, []byte{8, 1}, forma.PPS)

	pkts, err = enc.Encode([][]byte{{byte(h264.NALUTypeIDR)}}, 0)
	require.NoError(t, err)
	data = &UnitH264{RTPPackets: []*rtp.Packet{pkts[0]}}
	p.Process(data, true)

	require.Equal(t, [][]byte{
		{0x07, 4, 5, 6},
		{0x08, 1},
		{byte(h264.NALUTypeIDR)},
	}, data.AU)
}

func TestH264OversizedPackets(t *testing.T) {
	forma := &formats.H264{
		PayloadTyp:        96,
		SPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PacketizationMode: 1,
	}

	p, err := New(1472, forma, false, nil)
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
		data := &UnitH264{RTPPackets: []*rtp.Packet{pkt}}
		p.Process(data, false)
		out = append(out, data.RTPPackets...)
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
}

func TestH264EmptyPacket(t *testing.T) {
	forma := &formats.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	p, err := New(1472, forma, true, nil)
	require.NoError(t, err)

	unit := &UnitH264{
		AU: [][]byte{
			{0x07, 0x01, 0x02, 0x03}, // SPS
			{0x08, 0x01, 0x02},       // PPS
		},
	}

	p.Process(unit, false)

	// if all NALUs have been removed, no RTP packets must be generated.
	require.Equal(t, []*rtp.Packet(nil), unit.RTPPackets)
}

func TestH264KeyFrameWarning(t *testing.T) {
	forma := &formats.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	w := &testLogWriter{recv: make(chan string, 1)}
	p, err := New(1472, forma, true, w)
	require.NoError(t, err)

	ntp := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	err = p.Process(&UnitH264{
		AU: [][]byte{
			{0x01},
		},
		NTP: ntp,
	}, false)
	require.NoError(t, err)

	ntp = ntp.Add(30 * time.Second)
	err = p.Process(&UnitH264{
		AU: [][]byte{
			{0x01},
		},
		NTP: ntp,
	}, false)
	require.NoError(t, err)

	logl := <-w.recv
	require.Equal(t, "no H264 key frames received in 10s, stream can't be decoded", logl)
}
