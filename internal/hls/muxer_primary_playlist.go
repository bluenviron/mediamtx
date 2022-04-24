package hls

import (
	"encoding/hex"
	"io"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib"
)

type muxerPrimaryPlaylist struct {
	version    int
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackAAC
}

func newMuxerPrimaryPlaylist(
	version int,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *muxerPrimaryPlaylist {
	return &muxerPrimaryPlaylist{
		version:    version,
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}
}

func (p *muxerPrimaryPlaylist) reader() io.Reader {
	return &asyncReader{generator: func() []byte {
		var codecs []string

		if p.videoTrack != nil {
			sps := p.videoTrack.SPS()
			if len(sps) >= 4 {
				codecs = append(codecs, "avc1."+hex.EncodeToString(sps[1:4]))
			}
		}

		// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
		if p.audioTrack != nil {
			codecs = append(codecs, "mp4a.40."+strconv.FormatInt(int64(p.audioTrack.Type()), 10))
		}

		return []byte("#EXTM3U\n" +
			"#EXT-X-VERSION:" + strconv.FormatInt(int64(p.version), 10) + "\n" +
			"\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n" +
			"stream.m3u8\n")
	}}
}
