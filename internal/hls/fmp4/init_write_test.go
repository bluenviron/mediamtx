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

func TestInitWrite(t *testing.T) {
	t.Run("video + audio", func(t *testing.T) {
		byts, err := InitWrite(testVideoTrack, testAudioTrack)
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
		byts, err := InitWrite(testVideoTrack, nil)
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
		byts, err := InitWrite(nil, testAudioTrack)
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
