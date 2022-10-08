package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
)

type initReadState int

const (
	waitingTrak initReadState = iota
	waitingCodec
	waitingAVCC
)

// InitRead reads a FMP4 initialization file.
func InitRead(byts []byte) (*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio, error) {
	state := waitingTrak
	var videoTrack *gortsplib.TrackH264
	var audioTrack *gortsplib.TrackMPEG4Audio

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "trak":
			if state != waitingTrak {
				return nil, fmt.Errorf("parse error")
			}
			state = waitingCodec

		case "avc1":
			if state != waitingCodec {
				return nil, fmt.Errorf("parse error")
			}

			if videoTrack != nil {
				return nil, fmt.Errorf("multiple video tracks are not supported")
			}

			state = waitingAVCC

		case "avcC":
			if state != waitingAVCC {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			conf := box.(*gomp4.AVCDecoderConfiguration)

			if len(conf.SequenceParameterSets) > 1 {
				return nil, fmt.Errorf("multiple SPS are not supported")
			}

			var sps []byte
			if len(conf.SequenceParameterSets) == 1 {
				sps = conf.SequenceParameterSets[0].NALUnit
			}

			if len(conf.PictureParameterSets) > 1 {
				return nil, fmt.Errorf("multiple PPS are not supported")
			}

			var pps []byte
			if len(conf.PictureParameterSets) == 1 {
				pps = conf.PictureParameterSets[0].NALUnit
			}

			videoTrack = &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         sps,
				PPS:         pps,
			}

			state = waitingTrak

		case "mp4a":
			if state != waitingCodec {
				return nil, fmt.Errorf("parse error")
			}

			if audioTrack != nil {
				return nil, fmt.Errorf("multiple audio tracks are not supported")
			}

			return nil, fmt.Errorf("TODO: MP4a")
		}

		return h.Expand()
	})
	if err != nil {
		return nil, nil, err
	}

	if state != waitingTrak {
		return nil, nil, fmt.Errorf("parse error")
	}

	return videoTrack, audioTrack, nil
}
