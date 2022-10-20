//nolint:dupl
package fmp4

import (
	"bytes"
	"testing"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/stretchr/testify/require"
)

func testMP4(t *testing.T, byts []byte, boxes []gomp4.BoxPath) {
	i := 0
	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		require.Equal(t, boxes[i], h.Path)
		i++
		return h.Expand()
	})
	require.NoError(t, err)
}

var testSPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

var testVideoTrack = &gortsplib.TrackH264{
	PayloadType: 96,
	SPS:         testSPS,
	PPS:         []byte{0x08},
}

var testAudioTrack = &gortsplib.TrackMPEG4Audio{
	PayloadType: 97,
	Config: &mpeg4audio.Config{
		Type:         2,
		SampleRate:   44100,
		ChannelCount: 2,
	},
	SizeLength:       13,
	IndexLength:      3,
	IndexDeltaLength: 3,
}

func TestInitMarshal(t *testing.T) {
	t.Run("video + audio", func(t *testing.T) {
		init := Init{
			VideoTrack: &InitTrack{
				ID:        1,
				TimeScale: 90000,
				Track:     testVideoTrack,
			},
			AudioTrack: &InitTrack{
				ID:        2,
				TimeScale: uint32(testAudioTrack.ClockRate()),
				Track:     testAudioTrack,
			},
		}

		byts, err := init.Marshal()
		require.NoError(t, err)

		boxes := []gomp4.BoxPath{
			{gomp4.BoxTypeFtyp()},
			{gomp4.BoxTypeMoov()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(), gomp4.BoxTypeVmhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(), gomp4.BoxTypeDinf()},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeAvcC(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeBtrt(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
			},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeSmhd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeEsds(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeBtrt(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
			},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
		}
		testMP4(t, byts, boxes)
	})

	t.Run("video only", func(t *testing.T) {
		init := Init{
			VideoTrack: &InitTrack{
				ID:        1,
				TimeScale: 90000,
				Track:     testVideoTrack,
			},
		}

		byts, err := init.Marshal()
		require.NoError(t, err)

		boxes := []gomp4.BoxPath{
			{gomp4.BoxTypeFtyp()},
			{gomp4.BoxTypeMoov()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeVmhd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeAvcC(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeBtrt(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
			},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
		}
		testMP4(t, byts, boxes)
	})

	t.Run("audio only", func(t *testing.T) {
		init := &Init{
			AudioTrack: &InitTrack{
				ID:        1,
				TimeScale: uint32(testAudioTrack.ClockRate()),
				Track:     testAudioTrack,
			},
		}

		byts, err := init.Marshal()
		require.NoError(t, err)

		boxes := []gomp4.BoxPath{
			{gomp4.BoxTypeFtyp()},
			{gomp4.BoxTypeMoov()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeSmhd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeEsds(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeBtrt(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
			},
			{
				gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
				gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
			},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex()},
			{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
		}
		testMP4(t, byts, boxes)
	})
}

func TestInitUnmarshal(t *testing.T) {
	byts := []byte{
		0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70,
		0x64, 0x61, 0x73, 0x68, 0x00, 0x00, 0x00, 0x01,
		0x69, 0x73, 0x6f, 0x6d, 0x61, 0x76, 0x63, 0x31,
		0x64, 0x61, 0x73, 0x68, 0x00, 0x00, 0x02, 0x92,
		0x6d, 0x6f, 0x6f, 0x76, 0x00, 0x00, 0x00, 0x6c,
		0x6d, 0x76, 0x68, 0x64, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x98, 0x96, 0x80, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
		0x00, 0x00, 0x01, 0xf6, 0x74, 0x72, 0x61, 0x6b,
		0x00, 0x00, 0x00, 0x5c, 0x74, 0x6b, 0x68, 0x64,
		0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x40, 0x00, 0x00, 0x00, 0x03, 0xc0, 0x00, 0x00,
		0x02, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x92,
		0x6d, 0x64, 0x69, 0x61, 0x00, 0x00, 0x00, 0x20,
		0x6d, 0x64, 0x68, 0x64, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x98, 0x96, 0x80, 0x00, 0x00, 0x00, 0x00,
		0x55, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x38,
		0x68, 0x64, 0x6c, 0x72, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x76, 0x69, 0x64, 0x65,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x42, 0x72, 0x6f, 0x61,
		0x64, 0x70, 0x65, 0x61, 0x6b, 0x20, 0x56, 0x69,
		0x64, 0x65, 0x6f, 0x20, 0x48, 0x61, 0x6e, 0x64,
		0x6c, 0x65, 0x72, 0x00, 0x00, 0x00, 0x01, 0x32,
		0x6d, 0x69, 0x6e, 0x66, 0x00, 0x00, 0x00, 0x14,
		0x76, 0x6d, 0x68, 0x64, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x24, 0x64, 0x69, 0x6e, 0x66,
		0x00, 0x00, 0x00, 0x1c, 0x64, 0x72, 0x65, 0x66,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x0c, 0x75, 0x72, 0x6c, 0x20,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xf2,
		0x73, 0x74, 0x62, 0x6c, 0x00, 0x00, 0x00, 0xa6,
		0x73, 0x74, 0x73, 0x64, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x96,
		0x61, 0x76, 0x63, 0x31, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x03, 0xc0, 0x02, 0x1c,
		0x00, 0x48, 0x00, 0x00, 0x00, 0x48, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x04, 0x68,
		0x32, 0x36, 0x34, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18,
		0xff, 0xff, 0x00, 0x00, 0x00, 0x30, 0x61, 0x76,
		0x63, 0x43, 0x01, 0x42, 0xc0, 0x1f, 0xff, 0xe1,
		0x00, 0x19, 0x67, 0x42, 0xc0, 0x1f, 0xd9, 0x00,
		0xf0, 0x11, 0x7e, 0xf0, 0x11, 0x00, 0x00, 0x03,
		0x00, 0x01, 0x00, 0x00, 0x03, 0x00, 0x30, 0x8f,
		0x18, 0x32, 0x48, 0x01, 0x00, 0x04, 0x68, 0xcb,
		0x8c, 0xb2, 0x00, 0x00, 0x00, 0x10, 0x70, 0x61,
		0x73, 0x70, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x10, 0x73, 0x74,
		0x74, 0x73, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x73, 0x74,
		0x73, 0x63, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x14, 0x73, 0x74,
		0x73, 0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x10, 0x73, 0x74, 0x63, 0x6f, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x28, 0x6d, 0x76, 0x65, 0x78, 0x00, 0x00,
		0x00, 0x20, 0x74, 0x72, 0x65, 0x78, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	var init Init
	err := init.Unmarshal(byts)
	require.NoError(t, err)

	require.Equal(t, Init{
		VideoTrack: &InitTrack{
			ID:        256,
			TimeScale: 10000000,
			Track: &gortsplib.TrackH264{
				PayloadType: 96,
				SPS: []byte{
					0x67, 0x42, 0xc0, 0x1f, 0xd9, 0x00, 0xf0, 0x11,
					0x7e, 0xf0, 0x11, 0x00, 0x00, 0x03, 0x00, 0x01,
					0x00, 0x00, 0x03, 0x00, 0x30, 0x8f, 0x18, 0x32,
					0x48,
				},
				PPS: []byte{
					0x68, 0xcb, 0x8c, 0xb2,
				},
			},
		},
	}, init)
}
