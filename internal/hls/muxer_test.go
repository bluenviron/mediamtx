package hls

import (
	"bytes"
	"context"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/asticode/go-astits"
	"github.com/stretchr/testify/require"
)

// baseline profile without POC
var testSPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

func TestMuxerVideoAudio(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	audioTrack := &gortsplib.TrackAAC{
		PayloadType: 97,
		Config: &aac.MPEG4AudioConfig{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, audioTrack)
	require.NoError(t, err)
	defer m.Close()

	// group without IDR
	err = m.WriteH264(1*time.Second, [][]byte{
		{0x06},
		{0x07},
	})
	require.NoError(t, err)

	// group with IDR
	err = m.WriteH264(2*time.Second, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	err = m.WriteAAC(3*time.Second, [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	// group without IDR
	err = m.WriteH264(4*time.Second, [][]byte{
		{1}, // non-IDR
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	// group with IDR
	err = m.WriteH264(6*time.Second, [][]byte{
		{5}, // IDR
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.File("index.m3u8", "", "", "").Body)
	require.NoError(t, err)

	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:3\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028,mp4a.40.2\"\n"+
		"stream.m3u8\n", string(byts))

	byts, err = ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:4\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:4,\n` +
		`([0-9]+\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	dem := astits.NewDemuxer(context.Background(), m.File(ma[2], "", "", "").Body,
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
			0x05, 0x21, 0x00, 0x03, 0x19, 0x41, 0x00, 0x00,
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
			Length:                147,
			StuffingLength:        146,
			RandomAccessIndicator: true,
		},
		Header: &astits.PacketHeader{
			HasAdaptationField:        true,
			HasPayload:                true,
			PayloadUnitStartIndicator: true,
			PID:                       257,
		},
		Payload: []byte{
			0x00, 0x00, 0x01, 0xc0, 0x00, 0x1e, 0x80, 0x80,
			0x05, 0x21, 0x00, 0x07, 0xd8, 0x5f, 0xff, 0xf1,
			0x50, 0x80, 0x01, 0x7f, 0xfc, 0x01, 0x02, 0x03,
			0x04, 0xff, 0xf1, 0x50, 0x80, 0x01, 0x7f, 0xfc,
			0x05, 0x06, 0x07, 0x08,
		},
	}, pkt)
}

func TestMuxerVideoOnly(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	// group with IDR
	err = m.WriteH264(2*time.Second, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	// group with IDR
	err = m.WriteH264(6*time.Second, [][]byte{
		{5}, // IDR
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.File("index.m3u8", "", "", "").Body)
	require.NoError(t, err)

	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:3\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028\"\n"+
		"stream.m3u8\n", string(byts))

	byts, err = ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:4\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:4,\n` +
		`([0-9]+\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	dem := astits.NewDemuxer(context.Background(), m.File(ma[2], "", "", "").Body,
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
}

func TestMuxerAudioOnly(t *testing.T) {
	audioTrack := &gortsplib.TrackAAC{
		PayloadType: 97,
		Config: &aac.MPEG4AudioConfig{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, nil, audioTrack)
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 100; i++ {
		err = m.WriteAAC(1*time.Second, [][]byte{
			{0x01, 0x02, 0x03, 0x04},
		})
		require.NoError(t, err)
	}

	err = m.WriteAAC(2*time.Second, [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	err = m.WriteAAC(3*time.Second, [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.File("index.m3u8", "", "", "").Body)
	require.NoError(t, err)

	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:3\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"mp4a.40.2\"\n"+
		"stream.m3u8\n", string(byts))

	byts, err = ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:1\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:1,\n` +
		`([0-9]+\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	dem := astits.NewDemuxer(context.Background(), m.File(ma[2], "", "", "").Body,
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
}

func TestMuxerCloseBeforeFirstSegmentReader(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)

	// group with IDR
	err = m.WriteH264(2*time.Second, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	m.Close()

	b := m.File("stream.m3u8", "", "", "").Body
	require.Equal(t, nil, b)
}

func TestMuxerMaxSegmentSize(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 0, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(2*time.Second, [][]byte{
		testSPS,
		{5}, // IDR
	})
	require.EqualError(t, err, "reached maximum segment size")
}

func TestMuxerDoubleRead(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(2*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:2,\n` +
		`([0-9]+\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	byts1, err := ioutil.ReadAll(m.File(ma[2], "", "", "").Body)
	require.NoError(t, err)

	byts2, err := ioutil.ReadAll(m.File(ma[2], "", "", "").Body)
	require.NoError(t, err)
	require.Equal(t, byts1, byts2)
}
