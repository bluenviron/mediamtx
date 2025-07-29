package mpegts

import (
	"io"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediacommon/v2/pkg/rewindablereader"
)

// EnhancedReader is a mpegts.Reader wrapper
// That provides additional informations that are needed in order
// to perform conversion to RTSP.
type EnhancedReader struct {
	R io.Reader

	*mcmpegts.Reader

	latmConfigs map[uint16]*mpeg4audio.StreamMuxConfig
}

// Initialize initializes EnhancedReader.
func (r *EnhancedReader) Initialize() error {
	rr := &rewindablereader.Reader{R: r.R}
	mr := &mcmpegts.Reader{R: rr}
	err := mr.Initialize()
	if err != nil {
		return err
	}

	r.latmConfigs = make(map[uint16]*mpeg4audio.StreamMuxConfig)
	tracksToParse := 0

	for _, track := range mr.Tracks() {
		if _, ok := track.Codec.(*mcmpegts.CodecMPEG4AudioLATM); ok {
			cpid := track.PID
			done := false
			tracksToParse++

			mr.OnDataMPEG4AudioLATM(track, func(_ int64, els [][]byte) error {
				if done {
					return nil
				}

				var ame mpeg4audio.AudioMuxElement
				ame.MuxConfigPresent = true
				err2 := ame.Unmarshal(els[0])
				if err2 != nil {
					return nil //nolint:nilerr
				}

				if ame.MuxConfigPresent {
					r.latmConfigs[cpid] = ame.StreamMuxConfig
					tracksToParse--
					done = true
				}

				return nil
			})
		}
	}

	for tracksToParse > 0 {
		err = mr.Read()
		if err != nil {
			return err
		}
	}

	rr.Rewind()
	r.Reader = &mcmpegts.Reader{R: rr}
	err = r.Reader.Initialize()
	if err != nil {
		return err
	}

	return err
}
