package hls

import (
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/stretchr/testify/require"
)

var testTime = time.Date(2010, 0o1, 0o1, 0o1, 0o1, 0o1, 0, time.UTC)

// baseline profile without POC
var testSPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

func TestMuxerVideoAudio(t *testing.T) {
	videoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	audioTrack := &format.MPEG4Audio{
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

	for _, ca := range []string{
		"mpegts",
		"fmp4",
		"lowLatency",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			switch ca {
			case "mpegts":
				v = MuxerVariantMPEGTS

			case "fmp4":
				v = MuxerVariantFMP4

			case "lowLatency":
				v = MuxerVariantLowLatency
			}

			m, err := NewMuxer(v, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, audioTrack)
			require.NoError(t, err)
			defer m.Close()

			// access unit without IDR
			d := 1 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{0x06},
				{0x07},
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 2 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			d = 3500 * time.Millisecond
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			// access unit without IDR
			d = 4 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{1}, // non-IDR
			})
			require.NoError(t, err)

			d = 4500 * time.Millisecond
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 6 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 7 * time.Second
			err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, err := io.ReadAll(m.File("index.m3u8", "", "", "").Body)
			require.NoError(t, err)

			switch ca {
			case "mpegts":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028,mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))

			case "fmp4", "lowLatency":
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028,mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = io.ReadAll(m.File("stream.m3u8", "", "", "").Body)
			require.NoError(t, err)

			switch ca {
			case "mpegts":
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4,\n` +
					`(seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1,\n` +
					`(seg1\.ts)\n$`)
				ma := re.FindStringSubmatch(string(byts))
				require.NotEqual(t, 0, len(ma))

				seg := m.File(ma[2], "", "", "")
				require.Equal(t, http.StatusOK, seg.Status)
				_, err := io.ReadAll(seg.Body)
				require.NoError(t, err)

			case "fmp4":
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.mp4)\n$`)
				ma := re.FindStringSubmatch(string(byts))
				require.NotEqual(t, 0, len(ma))

				init := m.File("init.mp4", "", "", "")
				require.Equal(t, http.StatusOK, init.Status)
				_, err := io.ReadAll(init.Body)
				require.NoError(t, err)

				seg := m.File(ma[2], "", "", "")
				require.Equal(t, http.StatusOK, seg.Status)
				_, err = io.ReadAll(seg.Body)
				require.NoError(t, err)

			case "lowLatency":
				require.Equal(t,
					"#EXTM3U\n"+
						"#EXT-X-VERSION:9\n"+
						"#EXT-X-TARGETDURATION:4\n"+
						"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=5.00000,CAN-SKIP-UNTIL=24\n"+
						"#EXT-X-PART-INF:PART-TARGET=2\n"+
						"#EXT-X-MEDIA-SEQUENCE:2\n"+
						"#EXT-X-MAP:URI=\"init.mp4\"\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-GAP\n"+
						"#EXTINF:4.00000,\n"+
						"gap.mp4\n"+
						"#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:02Z\n"+
						"#EXT-X-PART:DURATION=2.00000,URI=\"part0.mp4\",INDEPENDENT=YES\n"+
						"#EXT-X-PART:DURATION=2.00000,URI=\"part1.mp4\"\n"+
						"#EXTINF:4.00000,\n"+
						"seg7.mp4\n"+
						"#EXT-X-PROGRAM-DATE-TIME:2010-01-01T01:01:06Z\n"+
						"#EXT-X-PART:DURATION=1.00000,URI=\"part3.mp4\",INDEPENDENT=YES\n"+
						"#EXTINF:1.00000,\n"+
						"seg8.mp4\n"+
						"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part4.mp4\"\n", string(byts))

				part := m.File("part3.mp4", "", "", "")
				require.Equal(t, http.StatusOK, part.Status)
				_, err = io.ReadAll(part.Body)
				require.NoError(t, err)

				recv := make(chan struct{})

				go func() {
					part = m.File("part4.mp4", "", "", "")
					_, err := io.ReadAll(part.Body)
					require.NoError(t, err)
					close(recv)
				}()

				d = 9 * time.Second
				err = m.WriteH26x(testTime.Add(d-1*time.Second), d, [][]byte{
					{1}, // non-IDR
				})
				require.NoError(t, err)

				<-recv
			}
		})
	}
}

func TestMuxerVideoOnly(t *testing.T) {
	videoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	for _, ca := range []string{
		"mpegts",
		"fmp4",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m, err := NewMuxer(v, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
			require.NoError(t, err)
			defer m.Close()

			// access unit with IDR
			d := 2 * time.Second
			err = m.WriteH26x(testTime.Add(d-2*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 6 * time.Second
			err = m.WriteH26x(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// access unit with IDR
			d = 7 * time.Second
			err = m.WriteH26x(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, err := io.ReadAll(m.File("index.m3u8", "", "", "").Body)
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028\"\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = io.ReadAll(m.File("stream.m3u8", "", "", "").Body)
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4,\n` +
					`(seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1,\n` +
					`(seg1\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" {
				_, err := io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)
			} else {
				_, err := io.ReadAll(m.File("init.mp4", "", "", "").Body)
				require.NoError(t, err)

				_, err = io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)
			}
		})
	}
}

func TestMuxerAudioOnly(t *testing.T) {
	audioTrack := &format.MPEG4Audio{
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

	for _, ca := range []string{
		"mpegts",
		"fmp4",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m, err := NewMuxer(v, 3, 1*time.Second, 0, 50*1024*1024, nil, audioTrack)
			require.NoError(t, err)
			defer m.Close()

			for i := 0; i < 100; i++ {
				d := 1 * time.Second
				err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
					0x01, 0x02, 0x03, 0x04,
				})
				require.NoError(t, err)
			}

			d := 2 * time.Second
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteAudio(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			byts, err := io.ReadAll(m.File("index.m3u8", "", "", "").Body)
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = io.ReadAll(m.File("stream.m3u8", "", "", "").Body)
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:1\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1,\n` +
					`(seg0\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:2\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:2.32200,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:0.02322,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" {
				_, err := io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)
			} else {
				_, err := io.ReadAll(m.File("init.mp4", "", "", "").Body)
				require.NoError(t, err)

				_, err = io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)
			}
		})
	}
}

func TestMuxerCloseBeforeFirstSegmentReader(t *testing.T) {
	videoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)

	// access unit with IDR
	err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
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
	videoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 0, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
		testSPS,
		{5}, // IDR
	})
	require.EqualError(t, err, "reached maximum segment size")
}

func TestMuxerDoubleRead(t *testing.T) {
	videoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH26x(testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH26x(testTime, 2*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	byts, err := io.ReadAll(m.File("stream.m3u8", "", "", "").Body)
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:2,\n` +
		`(seg0\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	byts1, err := io.ReadAll(m.File(ma[2], "", "", "").Body)
	require.NoError(t, err)

	byts2, err := io.ReadAll(m.File(ma[2], "", "", "").Body)
	require.NoError(t, err)
	require.Equal(t, byts1, byts2)
}

func TestMuxerFMP4ZeroDuration(t *testing.T) {
	videoTrack := &format.H264{
		PayloadTyp:        96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	m, err := NewMuxer(MuxerVariantLowLatency, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH26x(time.Now(), 0, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	err = m.WriteH26x(time.Now(), 1*time.Nanosecond, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)
}
