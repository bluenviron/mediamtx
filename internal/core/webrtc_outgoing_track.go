package core

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpav1"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtph264"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpvp8"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpvp9"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/webrtc/v3"
)

type webRTCOutgoingTrack struct {
	sender *webrtc.RTPSender
	media  *media.Media
	format formats.Format
	track  *webrtc.TrackLocalStaticRTP
	cb     func(formatprocessor.Unit, context.Context, chan error)
}

func newWebRTCOutgoingTrackVideo(medias media.Medias) (*webRTCOutgoingTrack, error) {
	var av1Format *formats.AV1
	av1Media := medias.FindFormat(&av1Format)

	if av1Format != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
			"av1",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpav1.Encoder{
			PayloadType:    105,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		encoder.Init()

		return &webRTCOutgoingTrack{
			media:  av1Media,
			format: av1Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				tunit := unit.(*formatprocessor.UnitAV1)

				if tunit.OBUs == nil {
					return
				}

				packets, err := encoder.Encode(tunit.OBUs, tunit.PTS)
				if err != nil {
					return
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	var vp9Format *formats.VP9
	vp9Media := medias.FindFormat(&vp9Format)

	if vp9Format != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP9,
				ClockRate: uint32(vp9Format.ClockRate()),
			},
			"vp9",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpvp9.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		encoder.Init()

		return &webRTCOutgoingTrack{
			media:  vp9Media,
			format: vp9Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				tunit := unit.(*formatprocessor.UnitVP9)

				if tunit.Frame == nil {
					return
				}

				packets, err := encoder.Encode(tunit.Frame, tunit.PTS)
				if err != nil {
					return
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	var vp8Format *formats.VP8
	vp8Media := medias.FindFormat(&vp8Format)

	if vp8Format != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: uint32(vp8Format.ClockRate()),
			},
			"vp8",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpvp8.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		encoder.Init()

		return &webRTCOutgoingTrack{
			media:  vp8Media,
			format: vp8Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				tunit := unit.(*formatprocessor.UnitVP8)

				if tunit.Frame == nil {
					return
				}

				packets, err := encoder.Encode(tunit.Frame, tunit.PTS)
				if err != nil {
					return
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	var h264Format *formats.H264
	h264Media := medias.FindFormat(&h264Format)

	if h264Format != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: uint32(h264Format.ClockRate()),
			},
			"h264",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtph264.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		encoder.Init()

		var lastPTS time.Duration
		firstNALUReceived := false

		return &webRTCOutgoingTrack{
			media:  h264Media,
			format: h264Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				tunit := unit.(*formatprocessor.UnitH264)

				if tunit.AU == nil {
					return
				}

				if !firstNALUReceived {
					firstNALUReceived = true
					lastPTS = tunit.PTS
				} else {
					if tunit.PTS < lastPTS {
						select {
						case writeError <- fmt.Errorf("WebRTC doesn't support H264 streams with B-frames"):
						case <-ctx.Done():
						}
						return
					}
					lastPTS = tunit.PTS
				}

				packets, err := encoder.Encode(tunit.AU, tunit.PTS)
				if err != nil {
					return
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	return nil, nil
}

func newWebRTCOutgoingTrackAudio(medias media.Medias) (*webRTCOutgoingTrack, error) {
	var opusFormat *formats.Opus
	opusMedia := medias.FindFormat(&opusFormat)

	if opusFormat != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: uint32(opusFormat.ClockRate()),
			},
			"opus",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  opusMedia,
			format: opusFormat,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				for _, pkt := range unit.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	var g722Format *formats.G722
	g722Media := medias.FindFormat(&g722Format)

	if g722Format != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: uint32(g722Format.ClockRate()),
			},
			"g722",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  g722Media,
			format: g722Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				for _, pkt := range unit.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	var g711Format *formats.G711
	g711Media := medias.FindFormat(&g711Format)

	if g711Format != nil {
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
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		return &webRTCOutgoingTrack{
			media:  g711Media,
			format: g711Format,
			track:  webRTCTrak,
			cb: func(unit formatprocessor.Unit, ctx context.Context, writeError chan error) {
				for _, pkt := range unit.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		}, nil
	}

	return nil, nil
}

func (t *webRTCOutgoingTrack) start() {
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
}
