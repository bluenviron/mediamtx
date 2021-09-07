package hls

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"

	"github.com/aler9/gortsplib"
)

type muxerPrimaryPlaylist struct {
	videoTrack *gortsplib.Track
	audioTrack *gortsplib.Track
	h264Conf   *gortsplib.TrackConfigH264

	cnt []byte
}

func newMuxerPrimaryPlaylist(
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track,
	h264Conf *gortsplib.TrackConfigH264,
) *muxerPrimaryPlaylist {
	p := &muxerPrimaryPlaylist{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		h264Conf:   h264Conf,
	}

	var codecs []string

	if p.videoTrack != nil {
		codecs = append(codecs, "avc1."+hex.EncodeToString(p.h264Conf.SPS[1:4]))
	}

	if p.audioTrack != nil {
		codecs = append(codecs, "mp4a.40.2")
	}

	p.cnt = []byte("#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n" +
		"stream.m3u8\n")

	return p
}

func (p *muxerPrimaryPlaylist) reader() io.Reader {
	return bytes.NewReader(p.cnt)
}
