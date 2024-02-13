package test

import (
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
)

// FormatH264 is a test H264 format.
var FormatH264 = &format.H264{
	PayloadTyp: 96,
	SPS: []byte{ // 1920x1080 baseline
		0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
		0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
	},
	PPS:               []byte{0x08, 0x06, 0x07, 0x08},
	PacketizationMode: 1,
}

// FormatMPEG4Audio is a test MPEG-4 audio format.
var FormatMPEG4Audio = &format.MPEG4Audio{
	PayloadTyp: 96,
	Config: &mpeg4audio.Config{
		Type:         2,
		SampleRate:   44100,
		ChannelCount: 2,
	},
	SizeLength:       13,
	IndexLength:      3,
	IndexDeltaLength: 3,
}
