package core

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpav1"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtph264"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpvp8"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpvp9"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type webRTCOutgoingTrack struct {
	sender *webrtc.RTPSender
	media  *media.Media
	format formats.Format
	track  *webrtc.TrackLocalStaticRTP
	cb     func(formatprocessor.Unit) error
}

func newWebRTCOutgoingTrackVideo(medias media.Medias) (*webRTCOutgoingTrack, error) {
	var av1Format *formats.AV1
	videoMedia := medias.FindFormat(&av1Format)

	if videoMedia != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
			"av1",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpav1.Encoder{
			PayloadType:    105,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err = encoder.Init()
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  videoMedia,
			format: av1Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				tunit := unit.(*formatprocessor.UnitAV1)

				if tunit.TU == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.TU, tunit.PTS)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var vp9Format *formats.VP9
	videoMedia = medias.FindFormat(&vp9Format)

	if videoMedia != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP9,
				ClockRate: uint32(vp9Format.ClockRate()),
			},
			"vp9",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpvp9.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err = encoder.Init()
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  videoMedia,
			format: vp9Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				tunit := unit.(*formatprocessor.UnitVP9)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame, tunit.PTS)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var vp8Format *formats.VP8
	videoMedia = medias.FindFormat(&vp8Format)

	if videoMedia != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: uint32(vp8Format.ClockRate()),
			},
			"vp8",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpvp8.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err = encoder.Init()
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  videoMedia,
			format: vp8Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				tunit := unit.(*formatprocessor.UnitVP8)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame, tunit.PTS)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var h264Format *formats.H264
	videoMedia = medias.FindFormat(&h264Format)

	if videoMedia != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: uint32(h264Format.ClockRate()),
			},
			"h264",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtph264.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err = encoder.Init()
		if err != nil {
			return nil, err
		}

		var lastPTS time.Duration
		firstNALUReceived := false

		return &webRTCOutgoingTrack{
			media:  videoMedia,
			format: h264Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				tunit := unit.(*formatprocessor.UnitH264)

				if tunit.AU == nil {
					return nil
				}

				if !firstNALUReceived {
					firstNALUReceived = true
					lastPTS = tunit.PTS
				} else {
					if tunit.PTS < lastPTS {
						return fmt.Errorf("WebRTC doesn't support H264 streams with B-frames")
					}
					lastPTS = tunit.PTS
				}

				packets, err := encoder.Encode(tunit.AU, tunit.PTS)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	return nil, nil
}

func newWebRTCOutgoingTrackAudio(medias media.Medias) (*webRTCOutgoingTrack, error) {
	var opusFormat *formats.Opus
	audioMedia := medias.FindFormat(&opusFormat)

	if audioMedia != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: uint32(opusFormat.ClockRate()),
				Channels:  2,
			},
			"opus",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  audioMedia,
			format: opusFormat,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				for _, pkt := range unit.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var g722Format *formats.G722
	audioMedia = medias.FindFormat(&g722Format)

	if audioMedia != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: uint32(g722Format.ClockRate()),
			},
			"g722",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  audioMedia,
			format: g722Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				for _, pkt := range unit.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var g711Format *formats.G711
	audioMedia = medias.FindFormat(&g711Format)

	if audioMedia != nil {
		var mtyp string
		if g711Format.MULaw {
			mtyp = webrtc.MimeTypePCMU
		} else {
			mtyp = webrtc.MimeTypePCMA
		}

		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  mtyp,
				ClockRate: uint32(g711Format.ClockRate()),
			},
			"g711",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  audioMedia,
			format: g711Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit) error {
				for _, pkt := range unit.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	return nil, nil
}

func (t *webRTCOutgoingTrack) start(
	ctx context.Context,
	r reader,
	stream *stream.Stream,
	ringBuffer *ringbuffer.RingBuffer,
	writeError chan error,
) {
	// read incoming RTCP packets to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			_, _, err := t.sender.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	stream.AddReader(r, t.media, t.format, func(unit formatprocessor.Unit) {
		ringBuffer.Push(func() {
			err := t.cb(unit)
			if err != nil {
				select {
				case writeError <- err:
				case <-ctx.Done():
				}
			}
		})
	})
}
