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
	waitingTkhd
	waitingMdhd
	waitingCodec
	waitingAVCC
)

// InitTrack is a track of a initialization file.
type InitTrack struct {
	ID        uint32
	TimeScale uint32
	Track     gortsplib.Track
}

// InitRead reads a FMP4 initialization file.
func InitRead(byts []byte) (*InitTrack, *InitTrack, error) {
	state := waitingTrak
	var curTrack *InitTrack
	var videoTrack *InitTrack
	var audioTrack *InitTrack

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "trak":
			if state != waitingTrak {
				return nil, fmt.Errorf("parse error")
			}

			curTrack = &InitTrack{}
			state = waitingTkhd

		case "tkhd":
			if state != waitingTkhd {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tkhd := box.(*gomp4.Tkhd)

			curTrack.ID = tkhd.TrackID
			state = waitingMdhd

		case "mdhd":
			if state != waitingMdhd {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd := box.(*gomp4.Mdhd)

			curTrack.TimeScale = mdhd.Timescale
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

			curTrack.Track = &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         sps,
				PPS:         pps,
			}
			videoTrack = curTrack
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

	if videoTrack == nil && audioTrack == nil {
		return nil, nil, fmt.Errorf("no tracks found")
	}

	return videoTrack, audioTrack, nil
}
