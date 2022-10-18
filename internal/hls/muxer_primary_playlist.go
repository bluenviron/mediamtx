package hls

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib"
)

type muxerPrimaryPlaylist struct {
	fmp4       bool
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackMPEG4Audio
}

func newMuxerPrimaryPlaylist(
	fmp4 bool,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
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
				sps := p.videoTrack.SafeSPS()
				if len(sps) >= 4 {
					codecs = append(codecs, "avc1."+hex.EncodeToString(sps[1:4]))
				}
			}

			// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
			if p.audioTrack != nil {
				codecs = append(codecs, "mp4a.40."+strconv.FormatInt(int64(p.audioTrack.Config.Type), 10))
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
