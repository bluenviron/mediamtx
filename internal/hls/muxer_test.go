package hls

import (
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/stretchr/testify/require"
)

func checkTSPacket(t *testing.T, byts []byte, pid int, afc int) {
	require.Equal(t, byte(0x47), byts[0])                                      // sync bit
	require.Equal(t, uint16(pid), (uint16(byts[1])<<8|uint16(byts[2]))&0x1fff) // PID
	require.Equal(t, uint8(afc), (byts[3]>>4)&0x03)                            // adaptation field control
}

func TestMuxerVideoAudio(t *testing.T) {
	videoTrack, err := gortsplib.NewTrackH264(96, []byte{0x07, 0x01, 0x02, 0x03}, []byte{0x08}, nil)
	require.NoError(t, err)

	audioTrack, err := gortsplib.NewTrackAAC(97, 2, 44100, 2, nil)
	require.NoError(t, err)

	m, err := NewMuxer(3, 1*time.Second, 50*1024*1024, videoTrack, audioTrack)
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
		{5}, // IDR
		{9}, // AUD
		{8}, // PPS
		{7}, // SPS
	})
	require.NoError(t, err)

	err = m.WriteAAC(3*time.Second, [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	// group without IDR
	err = m.WriteH264(4*time.Second, [][]byte{
		{6},
		{7},
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	// group with IDR
	err = m.WriteH264(6*time.Second, [][]byte{
		{5}, // IDR
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.PrimaryPlaylist())
	require.NoError(t, err)

	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.010203,mp4a.40.2\"\n"+
		"stream.m3u8\n", string(byts))

	byts, err = ioutil.ReadAll(m.StreamPlaylist())
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

	byts, err = ioutil.ReadAll(m.Segment(ma[2]))
	require.NoError(t, err)

	// PMT
	checkTSPacket(t, byts, 0, 1)
	byts = byts[188:]

	// PAT
	checkTSPacket(t, byts, 4096, 1)
	byts = byts[188:]

	// PES (H264)
	checkTSPacket(t, byts, 256, 3)
	byts = byts[164:]
	require.Equal(t,
		[]byte{
			0, 0, 0, 1, 9, 240, // AUD
			0, 0, 0, 1, 7, 1, 2, 3, // SPS
			0, 0, 0, 1, 8, // PPS
			0, 0, 0, 1, 5, // IDR
		},
		byts[:24],
	)
	byts = byts[24:]

	// PES (AAC)
	checkTSPacket(t, byts, 257, 3)
	byts = byts[166:]
	aus, err := aac.DecodeADTS(byts[:22])
	require.NoError(t, err)
	require.Equal(t, 2, len(aus))
	require.Equal(t, 2, aus[0].Type)
	require.Equal(t, 44100, aus[0].SampleRate)
	require.Equal(t, 2, aus[0].ChannelCount)
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, aus[0].AU)
	require.Equal(t, 2, aus[1].Type)
	require.Equal(t, 44100, aus[1].SampleRate)
	require.Equal(t, 2, aus[1].ChannelCount)
	require.Equal(t, []byte{0x05, 0x06, 0x07, 0x08}, aus[1].AU)
}

func TestMuxerAudio(t *testing.T) {
	audioTrack, err := gortsplib.NewTrackAAC(97, 2, 44100, 2, nil)
	require.NoError(t, err)

	m, err := NewMuxer(3, 1*time.Second, 50*1024*1024, nil, audioTrack)
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

	byts, err := ioutil.ReadAll(m.PrimaryPlaylist())
	require.NoError(t, err)

	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"mp4a.40.2\"\n"+
		"stream.m3u8\n", string(byts))

	byts, err = ioutil.ReadAll(m.StreamPlaylist())
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
}

func TestMuxerCloseBeforeFirstSegment(t *testing.T) {
	videoTrack, err := gortsplib.NewTrackH264(96, []byte{0x07, 0x01, 0x02, 0x03}, []byte{0x08}, nil)
	require.NoError(t, err)

	m, err := NewMuxer(3, 1*time.Second, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)

	// group with IDR
	err = m.WriteH264(2*time.Second, [][]byte{
		{5}, // IDR
		{9}, // AUD
		{8}, // PPS
		{7}, // SPS
	})
	require.NoError(t, err)

	m.Close()

	byts, err := ioutil.ReadAll(m.StreamPlaylist())
	require.NoError(t, err)
	require.Equal(t, []byte{}, byts)
}

func TestMuxerMaxSegmentSize(t *testing.T) {
	videoTrack, err := gortsplib.NewTrackH264(96, []byte{0x07, 0x01, 0x02, 0x03}, []byte{0x08}, nil)
	require.NoError(t, err)

	m, err := NewMuxer(3, 1*time.Second, 0, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(2*time.Second, [][]byte{
		{5},
	})
	require.EqualError(t, err, "reached maximum segment size")
}

func TestMuxerDoubleRead(t *testing.T) {
	videoTrack, err := gortsplib.NewTrackH264(96, []byte{0x07, 0x01, 0x02, 0x03}, []byte{0x08}, nil)
	require.NoError(t, err)

	m, err := NewMuxer(3, 1*time.Second, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(0, [][]byte{
		{5},
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(2*time.Second, [][]byte{
		{5},
		{2},
	})
	require.NoError(t, err)

	byts1, err := ioutil.ReadAll(m.streamPlaylist.segments[0].reader())
	require.NoError(t, err)

	byts2, err := ioutil.ReadAll(m.streamPlaylist.segments[0].reader())
	require.NoError(t, err)
	require.Equal(t, byts1, byts2)
}
