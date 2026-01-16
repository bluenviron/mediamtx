package stream

import (
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpac3"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpav1"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpfragmented"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpklv"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtplpcm"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmjpeg"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmpeg1audio"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmpeg1video"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmpeg4audio"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpsimpleaudio"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpvp8"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpvp9"
	mcopus "github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
)

func ptrOf[T any](v T) *T {
	return &v
}

type rtpEncoder interface {
	encode(unit.Payload) ([]*rtp.Packet, error)
}

type rtpEncoderH265 rtph265.Encoder

func (e *rtpEncoderH265) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtph265.Encoder)(e).Encode(payload.(unit.PayloadH265))
}

type rtpEncoderH264 rtph264.Encoder

func (e *rtpEncoderH264) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtph264.Encoder)(e).Encode(payload.(unit.PayloadH264))
}

type rtpEncoderAV1 rtpav1.Encoder

func (e *rtpEncoderAV1) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpav1.Encoder)(e).Encode(payload.(unit.PayloadAV1))
}

type rtpEncoderVP9 rtpvp9.Encoder

func (e *rtpEncoderVP9) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpvp9.Encoder)(e).Encode(payload.(unit.PayloadVP9))
}

type rtpEncoderVP8 rtpvp8.Encoder

func (e *rtpEncoderVP8) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpvp8.Encoder)(e).Encode(payload.(unit.PayloadVP8))
}

type rtpEncoderMPEG4Video rtpfragmented.Encoder

func (e *rtpEncoderMPEG4Video) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpfragmented.Encoder)(e).Encode(payload.(unit.PayloadMPEG4Video))
}

type rtpEncoderMPEG1Video rtpmpeg1video.Encoder

func (e *rtpEncoderMPEG1Video) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpmpeg1video.Encoder)(e).Encode(payload.(unit.PayloadMPEG1Video))
}

type rtpEncoderMJPEG rtpmjpeg.Encoder

func (e *rtpEncoderMJPEG) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpmjpeg.Encoder)(e).Encode(payload.(unit.PayloadMJPEG))
}

type rtpEncoderOpus rtpsimpleaudio.Encoder

func (e *rtpEncoderOpus) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	pts := int64(0)
	packets := make([]*rtp.Packet, len(payload.(unit.PayloadOpus)))

	for i, packet := range payload.(unit.PayloadOpus) {
		pkt, err := (*rtpsimpleaudio.Encoder)(e).Encode(packet)
		if err != nil {
			return nil, err
		}

		pkt.Timestamp += uint32(pts)
		pts += mcopus.PacketDuration2(packet)
		packets[i] = pkt
	}

	return packets, nil
}

type rtpEncoderMPEG4Audio rtpmpeg4audio.Encoder

func (e *rtpEncoderMPEG4Audio) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpmpeg4audio.Encoder)(e).Encode(payload.(unit.PayloadMPEG4Audio))
}

type rtpEncoderMPEG4AudioLATM rtpfragmented.Encoder

func (e *rtpEncoderMPEG4AudioLATM) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpfragmented.Encoder)(e).Encode(payload.(unit.PayloadMPEG4AudioLATM))
}

type rtpEncoderMPEG1Audio rtpmpeg1audio.Encoder

func (e *rtpEncoderMPEG1Audio) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpmpeg1audio.Encoder)(e).Encode(payload.(unit.PayloadMPEG1Audio))
}

type rtpEncoderAC3 rtpac3.Encoder

func (e *rtpEncoderAC3) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpac3.Encoder)(e).Encode(payload.(unit.PayloadAC3))
}

type rtpEncoderG711 rtplpcm.Encoder

func (e *rtpEncoderG711) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtplpcm.Encoder)(e).Encode(payload.(unit.PayloadG711))
}

type rtpEncoderLPCM rtplpcm.Encoder

func (e *rtpEncoderLPCM) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtplpcm.Encoder)(e).Encode(payload.(unit.PayloadLPCM))
}

type rtpEncoderKLV rtpklv.Encoder

func (e *rtpEncoderKLV) encode(payload unit.Payload) ([]*rtp.Packet, error) {
	return (*rtpklv.Encoder)(e).Encode(payload.(unit.PayloadKLV))
}

func newRTPEncoder(
	forma format.Format,
	rtpMaxPayloadSize int,
	ssrc *uint32,
	initialSequenceNumber *uint16,
) (rtpEncoder, error) {
	switch forma := forma.(type) {
	case *format.H265:
		wrapped := &rtph265.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
			MaxDONDiff:            forma.MaxDONDiff,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderH265)(wrapped), nil

	case *format.H264:
		wrapped := &rtph264.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
			PacketizationMode:     forma.PacketizationMode,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderH264)(wrapped), nil

	case *format.AV1:
		wrapped := &rtpav1.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderAV1)(wrapped), nil

	case *format.VP9:
		wrapped := &rtpvp9.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
			InitialPictureID:      ptrOf(uint16(0x35af)),
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderVP9)(wrapped), nil

	case *format.VP8:
		wrapped := &rtpvp8.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderVP8)(wrapped), nil

	case *format.MPEG4Video:
		wrapped := &rtpfragmented.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderMPEG4Video)(wrapped), nil

	case *format.MPEG1Video:
		wrapped := &rtpmpeg1video.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderMPEG1Video)(wrapped), nil

	case *format.MJPEG:
		wrapped := &rtpmjpeg.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderMJPEG)(wrapped), nil

	case *format.Opus:
		wrapped := &rtpsimpleaudio.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderOpus)(wrapped), nil

	case *format.MPEG4Audio:
		wrapped := &rtpmpeg4audio.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
			SizeLength:            forma.SizeLength,
			IndexLength:           forma.IndexLength,
			IndexDeltaLength:      forma.IndexDeltaLength,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderMPEG4Audio)(wrapped), nil

	case *format.MPEG4AudioLATM:
		wrapped := &rtpfragmented.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderMPEG4AudioLATM)(wrapped), nil

	case *format.MPEG1Audio:
		wrapped := &rtpmpeg1audio.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderMPEG1Audio)(wrapped), nil

	case *format.AC3:
		wrapped := &rtpac3.Encoder{
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderAC3)(wrapped), nil

	case *format.G711:
		wrapped := &rtplpcm.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadType(),
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
			BitDepth:              8,
			ChannelCount:          forma.ChannelCount,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderG711)(wrapped), nil

	case *format.LPCM:
		wrapped := &rtplpcm.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
			BitDepth:              forma.BitDepth,
			ChannelCount:          forma.ChannelCount,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderLPCM)(wrapped), nil

	case *format.KLV:
		wrapped := &rtpklv.Encoder{
			PayloadMaxSize:        rtpMaxPayloadSize,
			PayloadType:           forma.PayloadTyp,
			SSRC:                  ssrc,
			InitialSequenceNumber: initialSequenceNumber,
		}
		err := wrapped.Init()
		if err != nil {
			return nil, err
		}

		return (*rtpEncoderKLV)(wrapped), nil

	default:
		return nil, nil
	}
}
