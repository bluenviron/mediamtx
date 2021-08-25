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
	h264Conf   *gortsplib.TrackConfigH264

	breader *bytes.Reader
}

func newPrimaryPlaylist(
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track,
	h264Conf *gortsplib.TrackConfigH264,
) *primaryPlaylist {
	p := &primaryPlaylist{
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

	cnt := "#EXTM3U\n"
	cnt += "#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n"
	cnt += "stream.m3u8\n"

	p.breader = bytes.NewReader([]byte(cnt))

	return p
}

func (p *primaryPlaylist) reader() io.Reader {
	return p.breader
}
