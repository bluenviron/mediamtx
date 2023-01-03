package hls

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/format"
)

func codecParameters(track format.Format) string {
	switch ttrack := track.(type) {
	case *format.H264:
		sps := ttrack.SafeSPS()
		if len(sps) >= 4 {
			return "avc1." + hex.EncodeToString(sps[1:4])
		}

	case *format.H265:
		var sps h265.SPS
		err := sps.Unmarshal(ttrack.SafeSPS())
		if err == nil {
			return "hvc1." + strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralProfileIdc), 10) +
				".4.L" + strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralLevelIdc), 10) + ".B0"
		}

	case *format.MPEG4Audio:
		// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
		return "mp4a.40." + strconv.FormatInt(int64(ttrack.Config.Type), 10)

	case *format.Opus:
		return "opus"
	}

	return ""
}

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
				codecs = append(codecs, codecParameters(p.videoTrack))
			}
			if p.audioTrack != nil {
				codecs = append(codecs, codecParameters(p.audioTrack))
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
