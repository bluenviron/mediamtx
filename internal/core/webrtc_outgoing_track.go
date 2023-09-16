package core

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpav1"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp8"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp9"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type webRTCOutgoingTrack struct {
	sender *webrtc.RTPSender
	media  *description.Media
	format format.Format
	track  *webrtc.TrackLocalStaticRTP
	cb     func(unit.Unit) error
}

func newWebRTCOutgoingTrackVideo(desc *description.Session) (*webRTCOutgoingTrack, error) {
	var av1Format *format.AV1
	videoMedia := desc.FindFormat(&av1Format)

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
			cb: func(u unit.Unit) error {
				tunit := u.(*unit.AV1)

				if tunit.TU == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.TU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp = tunit.RTPPackets[0].Timestamp
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var vp9Format *format.VP9
	videoMedia = desc.FindFormat(&vp9Format)

	if videoMedia != nil { //nolint:dupl
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
			cb: func(u unit.Unit) error {
				tunit := u.(*unit.VP9)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp = tunit.RTPPackets[0].Timestamp
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var vp8Format *format.VP8
	videoMedia = desc.FindFormat(&vp8Format)

	if videoMedia != nil { //nolint:dupl
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
			cb: func(u unit.Unit) error {
				tunit := u.(*unit.VP8)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp = tunit.RTPPackets[0].Timestamp
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var h264Format *format.H264
	videoMedia = desc.FindFormat(&h264Format)

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

		firstReceived := false
		var lastPTS time.Duration

		return &webRTCOutgoingTrack{
			media:  videoMedia,
			format: h264Format,
			track:  webRTCTrak,
			cb: func(u unit.Unit) error {
				tunit := u.(*unit.H264)

				if tunit.AU == nil {
					return nil
				}

				if !firstReceived {
					firstReceived = true
				} else if tunit.PTS < lastPTS {
					return fmt.Errorf("WebRTC doesn't support H264 streams with B-frames")
				}
				lastPTS = tunit.PTS

				packets, err := encoder.Encode(tunit.AU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp = tunit.RTPPackets[0].Timestamp
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	return nil, nil
}

func newWebRTCOutgoingTrackAudio(desc *description.Session) (*webRTCOutgoingTrack, error) {
	var opusFormat *format.Opus
	audioMedia := desc.FindFormat(&opusFormat)

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
			cb: func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var g722Format *format.G722
	audioMedia = desc.FindFormat(&g722Format)

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
			cb: func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	var g711Format *format.G711
	audioMedia = desc.FindFormat(&g711Format)

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
			cb: func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			},
		}, nil
	}

	return nil, nil
}

func (t *webRTCOutgoingTrack) start(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
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

	stream.AddReader(writer, t.media, t.format, t.cb)
}
