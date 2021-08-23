package hls

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"

	"github.com/aler9/gortsplib"
)

type primaryPlaylist struct {
	videoTrack *gortsplib.Track
	audioTrack *gortsplib.Track
	h264SPS    []byte
	h264PPS    []byte

	breader *bytes.Reader
}

func newPrimaryPlaylist(
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track,
	h264SPS []byte,
	h264PPS []byte,
) *primaryPlaylist {
	p := &primaryPlaylist{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		h264SPS:    h264SPS,
		h264PPS:    h264PPS,
	}

	var codecs []string

	if p.videoTrack != nil {
		codecs = append(codecs, "avc1."+hex.EncodeToString(p.h264SPS[1:4]))
	}

	if p.audioTrack != nil {
		codecs = append(codecs, "mp4a.40.2")
	}

	cnt := "#EXTM3U\n"
	cnt += "#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n"
	cnt += "stream.m3u8\n"

	p.breader = bytes.NewReader([]byte(cnt))

	return p
}

func (p *primaryPlaylist) reader() io.Reader {
	return p.breader
}
