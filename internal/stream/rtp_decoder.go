package stream

import (
	"errors"

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
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
)

type rtpDecoder interface {
	decode(*rtp.Packet) (unit.Payload, error)
}

type rtpDecoderAV1 rtpav1.Decoder

func (d *rtpDecoderAV1) decode(pkt *rtp.Packet) (unit.Payload, error) {
	tu, err := (*rtpav1.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpav1.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtpav1.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadAV1(tu), nil
}

type rtpDecoderVP9 rtpvp9.Decoder

func (d *rtpDecoderVP9) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frame, err := (*rtpvp9.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpvp9.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtpvp9.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadVP9(frame), nil
}

type rtpDecoderVP8 rtpvp8.Decoder

func (d *rtpDecoderVP8) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frame, err := (*rtpvp8.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpvp8.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtpvp8.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadVP8(frame), nil
}

type rtpDecoderH265 rtph265.Decoder

func (d *rtpDecoderH265) decode(pkt *rtp.Packet) (unit.Payload, error) {
	au, err := (*rtph265.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtph265.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtph265.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadH265(au), nil
}

type rtpDecoderH264 rtph264.Decoder

func (d *rtpDecoderH264) decode(pkt *rtp.Packet) (unit.Payload, error) {
	au, err := (*rtph264.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtph264.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadH264(au), nil
}

type rtpDecoderMPEG4Video rtpfragmented.Decoder

func (d *rtpDecoderMPEG4Video) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frame, err := (*rtpfragmented.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpfragmented.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadMPEG4Video(frame), nil
}

type rtpDecoderMPEG1Video rtpmpeg1video.Decoder

func (d *rtpDecoderMPEG1Video) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frame, err := (*rtpmpeg1video.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpmpeg1video.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtpmpeg1video.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadMPEG1Video(frame), nil
}

type rtpDecoderMJPEG rtpmjpeg.Decoder

func (d *rtpDecoderMJPEG) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frame, err := (*rtpmjpeg.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpmjpeg.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtpmjpeg.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadMJPEG(frame), nil
}

type rtpDecoderOpus rtpsimpleaudio.Decoder

func (d *rtpDecoderOpus) decode(pkt *rtp.Packet) (unit.Payload, error) {
	packet, err := (*rtpsimpleaudio.Decoder)(d).Decode(pkt)
	if err != nil {
		return nil, err
	}

	return unit.PayloadOpus{packet}, nil
}

type rtpDecoderMPEG4Audio rtpmpeg4audio.Decoder

func (d *rtpDecoderMPEG4Audio) decode(pkt *rtp.Packet) (unit.Payload, error) {
	aus, err := (*rtpmpeg4audio.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpmpeg4audio.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadMPEG4Audio(aus), nil
}

type rtpDecoderMPEG4AudioLATM rtpfragmented.Decoder

func (d *rtpDecoderMPEG4AudioLATM) decode(pkt *rtp.Packet) (unit.Payload, error) {
	payload, err := (*rtpfragmented.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpfragmented.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadMPEG4AudioLATM(payload), nil
}

type rtpDecoderMPEG1Audio rtpmpeg1audio.Decoder

func (d *rtpDecoderMPEG1Audio) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frames, err := (*rtpmpeg1audio.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpmpeg1audio.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtpmpeg1audio.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadMPEG1Audio(frames), nil
}

type rtpDecoderAC3 rtpac3.Decoder

func (d *rtpDecoderAC3) decode(pkt *rtp.Packet) (unit.Payload, error) {
	frames, err := (*rtpac3.Decoder)(d).Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpac3.ErrMorePacketsNeeded) {
			return nil, nil
		}
		return nil, err
	}

	return unit.PayloadAC3(frames), nil
}

type rtpDecoderG711 rtplpcm.Decoder

func (d *rtpDecoderG711) decode(pkt *rtp.Packet) (unit.Payload, error) {
	samples, err := (*rtplpcm.Decoder)(d).Decode(pkt)
	if err != nil {
		return nil, err
	}

	return unit.PayloadG711(samples), nil
}

type rtpDecoderLPCM rtplpcm.Decoder

func (d *rtpDecoderLPCM) decode(pkt *rtp.Packet) (unit.Payload, error) {
	samples, err := (*rtplpcm.Decoder)(d).Decode(pkt)
	if err != nil {
		return nil, err
	}

	return unit.PayloadLPCM(samples), nil
}

type rtpDecoderKLV rtpklv.Decoder

func (d *rtpDecoderKLV) decode(pkt *rtp.Packet) (unit.Payload, error) {
	payload, err := (*rtpklv.Decoder)(d).Decode(pkt)
	if err != nil {
		return nil, err
	}

	return unit.PayloadKLV(payload), nil
}

func newRTPDecoder(forma format.Format) (rtpDecoder, error) {
	switch forma := forma.(type) {
	case *format.AV1:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderAV1)(wrapped), nil

	case *format.VP9:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderVP9)(wrapped), nil

	case *format.VP8:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderVP8)(wrapped), nil

	case *format.H265:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderH265)(wrapped), nil

	case *format.H264:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderH264)(wrapped), nil

	case *format.MPEG4Video:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderMPEG4Video)(wrapped), nil

	case *format.MPEG1Video:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderMPEG1Video)(wrapped), nil

	case *format.MJPEG:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderMJPEG)(wrapped), nil

	case *format.Opus:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderOpus)(wrapped), nil

	case *format.MPEG4Audio:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderMPEG4Audio)(wrapped), nil

	case *format.MPEG4AudioLATM:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderMPEG4AudioLATM)(wrapped), nil

	case *format.MPEG1Audio:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderMPEG1Audio)(wrapped), nil

	case *format.AC3:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderAC3)(wrapped), nil

	case *format.G711:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderG711)(wrapped), nil

	case *format.LPCM:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderLPCM)(wrapped), nil

	case *format.KLV:
		wrapped, err := forma.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return (*rtpDecoderKLV)(wrapped), nil

	default:
		return nil, nil
	}
}
