package mpegts

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/asticode/go-astits"
	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	testSPS := []byte{
		0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
		0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
		0x20,
	}

	testVideoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	testAudioTrack := &format.MPEG4Audio{
		PayloadTyp: 97,
		Config: &mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}

	type videoSample struct {
		NALUs [][]byte
		PTS   time.Duration
		DTS   time.Duration
	}

	type audioSample struct {
		AU  []byte
		PTS time.Duration
	}

	type sample interface{}

	testSamples := []sample{
		videoSample{
			NALUs: [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			},
			PTS: 2 * time.Second,
			DTS: 2 * time.Second,
		},
		audioSample{
			AU: []byte{
				0x01, 0x02, 0x03, 0x04,
			},
			PTS: 3 * time.Second,
		},
		audioSample{
			AU: []byte{
				0x01, 0x02, 0x03, 0x04,
			},
			PTS: 3500 * time.Millisecond,
		},
		videoSample{
			NALUs: [][]byte{
				{1}, // non-IDR
			},
			PTS: 4 * time.Second,
			DTS: 4 * time.Second,
		},
		audioSample{
			AU: []byte{
				0x01, 0x02, 0x03, 0x04,
			},
			PTS: 4500 * time.Millisecond,
		},
		videoSample{
			NALUs: [][]byte{
				{1}, // non-IDR
			},
			PTS: 6 * time.Second,
			DTS: 6 * time.Second,
		},
	}

	t.Run("video + audio", func(t *testing.T) {
		w := NewWriter(testVideoTrack, testAudioTrack)

		for _, sample := range testSamples {
			switch tsample := sample.(type) {
			case videoSample:
				err := w.WriteH264(
					tsample.DTS-2*time.Second,
					tsample.DTS,
					tsample.PTS,
					h264.IDRPresent(tsample.NALUs),
					tsample.NALUs)
				require.NoError(t, err)

			case audioSample:
				err := w.WriteAAC(
					tsample.PTS-2*time.Second,
					tsample.PTS,
					tsample.AU)
				require.NoError(t, err)
			}
		}

		byts := w.GenerateSegment()

		dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts),
			astits.DemuxerOptPacketSize(188))

		// PMT
		pkt, err := dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			Header: &astits.PacketHeader{
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       0,
			},
			Payload: append([]byte{
				0x00, 0x00, 0xb0, 0x0d, 0x00, 0x00, 0xc1, 0x00,
				0x00, 0x00, 0x01, 0xf0, 0x00, 0x71, 0x10, 0xd8,
				0x78,
			}, bytes.Repeat([]byte{0xff}, 167)...),
		}, pkt)

		// PAT
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			Header: &astits.PacketHeader{
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       4096,
			},
			Payload: append([]byte{
				0x00, 0x02, 0xb0, 0x17, 0x00, 0x01, 0xc1, 0x00,
				0x00, 0xe1, 0x00, 0xf0, 0x00, 0x1b, 0xe1, 0x00,
				0xf0, 0x00, 0x0f, 0xe1, 0x01, 0xf0, 0x00, 0x2f,
				0x44, 0xb9, 0x9b,
			}, bytes.Repeat([]byte{0xff}, 157)...),
		}, pkt)

		// PES (H264)
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			AdaptationField: &astits.PacketAdaptationField{
				Length:                124,
				StuffingLength:        117,
				HasPCR:                true,
				PCR:                   &astits.ClockReference{},
				RandomAccessIndicator: true,
			},
			Header: &astits.PacketHeader{
				HasAdaptationField:        true,
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       256,
			},
			Payload: []byte{
				0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x80,
				0x05, 0x21, 0x00, 0x0d, 0x97, 0x81, 0x00, 0x00,
				0x00, 0x01, 0x09, 0xf0, 0x00, 0x00, 0x00, 0x01,
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
				0x20, 0x00, 0x00, 0x00, 0x01, 0x08, 0x00, 0x00,
				0x00, 0x01, 0x05,
			},
		}, pkt)

		// PES (AAC)
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			AdaptationField: &astits.PacketAdaptationField{
				Length:                158,
				StuffingLength:        157,
				RandomAccessIndicator: true,
			},
			Header: &astits.PacketHeader{
				HasAdaptationField:        true,
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       257,
			},
			Payload: []byte{
				0x00, 0x00, 0x01, 0xc0, 0x00, 0x13, 0x80, 0x80,
				0x05, 0x21, 0x00, 0x13, 0x56, 0xa1, 0xff, 0xf1,
				0x50, 0x80, 0x01, 0x7f, 0xfc, 0x01, 0x02, 0x03,
				0x04,
			},
		}, pkt)
	})

	t.Run("video only", func(t *testing.T) {
		w := NewWriter(testVideoTrack, nil)

		for _, sample := range testSamples {
			if tsample, ok := sample.(videoSample); ok {
				err := w.WriteH264(
					tsample.DTS-2*time.Second,
					tsample.DTS,
					tsample.PTS,
					h264.IDRPresent(tsample.NALUs),
					tsample.NALUs)
				require.NoError(t, err)
			}
		}

		byts := w.GenerateSegment()

		dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts),
			astits.DemuxerOptPacketSize(188))

		// PMT
		pkt, err := dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			Header: &astits.PacketHeader{
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       0,
			},
			Payload: append([]byte{
				0x00, 0x00, 0xb0, 0x0d, 0x00, 0x00, 0xc1, 0x00,
				0x00, 0x00, 0x01, 0xf0, 0x00, 0x71, 0x10, 0xd8,
				0x78,
			}, bytes.Repeat([]byte{0xff}, 167)...),
		}, pkt)

		// PAT
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			Header: &astits.PacketHeader{
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       4096,
			},
			Payload: append([]byte{
				0x00, 0x02, 0xb0, 0x12, 0x00, 0x01, 0xc1, 0x00,
				0x00, 0xe1, 0x00, 0xf0, 0x00, 0x1b, 0xe1, 0x00,
				0xf0, 0x00, 0x15, 0xbd, 0x4d, 0x56,
			}, bytes.Repeat([]byte{0xff}, 162)...),
		}, pkt)

		// PES (H264)
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			AdaptationField: &astits.PacketAdaptationField{
				Length:                124,
				StuffingLength:        117,
				HasPCR:                true,
				PCR:                   &astits.ClockReference{},
				RandomAccessIndicator: true,
			},
			Header: &astits.PacketHeader{
				HasAdaptationField:        true,
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       256,
			},
			Payload: []byte{
				0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x80,
				0x05, 0x21, 0x00, 0x0d, 0x97, 0x81, 0x00, 0x00,
				0x00, 0x01, 0x09, 0xf0, 0x00, 0x00, 0x00, 0x01,
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
				0x20, 0x00, 0x00, 0x00, 0x01, 0x08, 0x00, 0x00,
				0x00, 0x01, 0x05,
			},
		}, pkt)
	})

	t.Run("audio only", func(t *testing.T) {
		w := NewWriter(nil, testAudioTrack)

		for _, sample := range testSamples {
			if tsample, ok := sample.(audioSample); ok {
				err := w.WriteAAC(
					tsample.PTS-2*time.Second,
					tsample.PTS,
					tsample.AU)
				require.NoError(t, err)
			}
		}

		byts := w.GenerateSegment()

		dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts),
			astits.DemuxerOptPacketSize(188))

		// PMT
		pkt, err := dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			Header: &astits.PacketHeader{
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       0,
			},
			Payload: append([]byte{
				0x00, 0x00, 0xb0, 0x0d, 0x00, 0x00, 0xc1, 0x00,
				0x00, 0x00, 0x01, 0xf0, 0x00, 0x71, 0x10, 0xd8,
				0x78,
			}, bytes.Repeat([]byte{0xff}, 167)...),
		}, pkt)

		// PAT
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			Header: &astits.PacketHeader{
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       4096,
			},
			Payload: append([]byte{
				0x00, 0x02, 0xb0, 0x12, 0x00, 0x01, 0xc1, 0x00,
				0x00, 0xe1, 0x01, 0xf0, 0x00, 0x0f, 0xe1, 0x01,
				0xf0, 0x00, 0xec, 0xe2, 0xb0, 0x94,
			}, bytes.Repeat([]byte{0xff}, 162)...),
		}, pkt)

		// PES (AAC)
		pkt, err = dem.NextPacket()
		require.NoError(t, err)
		require.Equal(t, &astits.Packet{
			AdaptationField: &astits.PacketAdaptationField{
				Length:                158,
				StuffingLength:        151,
				RandomAccessIndicator: true,
				HasPCR:                true,
				PCR:                   &astits.ClockReference{Base: 90000},
			},
			Header: &astits.PacketHeader{
				HasAdaptationField:        true,
				HasPayload:                true,
				PayloadUnitStartIndicator: true,
				PID:                       257,
			},
			Payload: []byte{
				0x00, 0x00, 0x01, 0xc0, 0x00, 0x13, 0x80, 0x80,
				0x05, 0x21, 0x00, 0x13, 0x56, 0xa1, 0xff, 0xf1,
				0x50, 0x80, 0x01, 0x7f, 0xfc, 0x01, 0x02, 0x03,
				0x04,
			},
		}, pkt)
	})
}
