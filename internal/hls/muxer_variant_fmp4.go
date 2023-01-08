package hls

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

func extractVideoParams(track format.Format) [][]byte {
	switch ttrack := track.(type) {
	case *format.H264:
		params := make([][]byte, 2)
		params[0] = ttrack.SafeSPS()
		params[1] = ttrack.SafePPS()
		return params

	case *format.H265:
		params := make([][]byte, 3)
		params[0] = ttrack.SafeVPS()
		params[1] = ttrack.SafeSPS()
		params[2] = ttrack.SafePPS()
		return params

	default:
		return nil
	}
}

func videoParamsEqual(p1 [][]byte, p2 [][]byte) bool {
	if len(p1) != len(p2) {
		return true
	}

	for i, p := range p1 {
		if !bytes.Equal(p2[i], p) {
			return false
		}
	}
	return true
}

type muxerVariantFMP4 struct {
	playlist   *muxerVariantFMP4Playlist
	segmenter  *muxerVariantFMP4Segmenter
	videoTrack format.Format
	audioTrack format.Format

	mutex           sync.Mutex
	lastVideoParams [][]byte
	initContent     []byte
}

func newMuxerVariantFMP4(
	lowLatency bool,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack format.Format,
	audioTrack format.Format,
) *muxerVariantFMP4 {
	v := &muxerVariantFMP4{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}

	v.playlist = newMuxerVariantFMP4Playlist(
		lowLatency,
		segmentCount,
		videoTrack,
		audioTrack,
	)

	v.segmenter = newMuxerVariantFMP4Segmenter(
		lowLatency,
		segmentCount,
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
		v.playlist.onSegmentFinalized,
		v.playlist.onPartFinalized,
	)

	return v
}

func (v *muxerVariantFMP4) close() {
	v.playlist.close()
}

func (v *muxerVariantFMP4) writeH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	return v.segmenter.writeH26x(ntp, pts, au)
}

func (v *muxerVariantFMP4) writeAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return v.segmenter.writeAudio(ntp, pts, au)
}

func (v *muxerVariantFMP4) mustRegenerateInit() bool {
	if v.videoTrack == nil {
		return false
	}

	videoParams := extractVideoParams(v.videoTrack)
	if !videoParamsEqual(videoParams, v.lastVideoParams) {
		v.lastVideoParams = videoParams
		return true
	}

	return false
}

func (v *muxerVariantFMP4) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "init.mp4" {
		v.mutex.Lock()
		defer v.mutex.Unlock()

		if v.initContent == nil || v.mustRegenerateInit() {
			init := fmp4.Init{}
			trackID := 1

			if v.videoTrack != nil {
				init.Tracks = append(init.Tracks, &fmp4.InitTrack{
					ID:        trackID,
					TimeScale: 90000,
					Format:    v.videoTrack,
				})
				trackID++
			}

			if v.audioTrack != nil {
				init.Tracks = append(init.Tracks, &fmp4.InitTrack{
					ID:        trackID,
					TimeScale: uint32(v.audioTrack.ClockRate()),
					Format:    v.audioTrack,
				})
			}

			var err error
			v.initContent, err = init.Marshal()
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusNotFound}
			}
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "video/mp4",
			},
			Body: bytes.NewReader(v.initContent),
		}
	}

	return v.playlist.file(name, msn, part, skip)
}
