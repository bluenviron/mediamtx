package hls

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib/v2/pkg/format"
)

type muxerPrimaryPlaylist struct {
	fmp4       bool
	videoTrack format.Format
	audioTrack format.Format
}

func newMuxerPrimaryPlaylist(
	fmp4 bool,
	videoTrack format.Format,
	audioTrack format.Format,
) *muxerPrimaryPlaylist {
	return &muxerPrimaryPlaylist{
		fmp4:       fmp4,
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}
}

func (p *muxerPrimaryPlaylist) file() *MuxerFileResponse {
	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `application/x-mpegURL`,
		},
		Body: func() io.Reader {
			var codecs []string

			if p.videoTrack != nil {
				codecs = append(codecs, codecParametersGenerate(p.videoTrack))
			}
			if p.audioTrack != nil {
				codecs = append(codecs, codecParametersGenerate(p.audioTrack))
			}

			var version int
			if !p.fmp4 {
				version = 3
			} else {
				version = 9
			}

			return bytes.NewReader([]byte("#EXTM3U\n" +
				"#EXT-X-VERSION:" + strconv.FormatInt(int64(version), 10) + "\n" +
				"#EXT-X-INDEPENDENT-SEGMENTS\n" +
				"\n" +
				"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n" +
				"stream.m3u8\n"))
		}(),
	}
}
