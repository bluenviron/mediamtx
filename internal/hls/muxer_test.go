package hls

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"
)

func TestMuxer(t *testing.T) {
	videoTrack, err := gortsplib.NewTrackH264(96, []byte{0x01, 0x02, 0x03, 0x04}, []byte{0x01, 0x02, 0x03, 0x04})
	require.NoError(t, err)

	audioTrack, err := gortsplib.NewTrackAAC(97, []byte{17, 144})
	require.NoError(t, err)

	m, err := NewMuxer(3, 5*time.Second, videoTrack, audioTrack)
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
		{0x05},
		{0x06},
	})
	require.NoError(t, err)

	err = m.WriteAAC(3*time.Second, [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	// group without IDR
	err = m.WriteH264(4*time.Second, [][]byte{
		{0x06},
		{0x07},
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.Playlist())
	require.NoError(t, err)

	require.Regexp(t, `^#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-ALLOW-CACHE:NO\n#EXT-X-TARGETDURATION:5\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\n[0-9]+\.ts\n$`, string(byts))
}
