package codecprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestMPEG4VideoProcessUnit(t *testing.T) {
	forma := &format.MPEG4Video{
		PayloadTyp: 96,
	}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	u1 := &unit.Unit{
		PTS: 30000,
		Payload: unit.PayloadMPEG4Video{
			0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode),
			0, 0, 1, 0xFF,
			0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode),
			0, 0, 1, 0xF0,
		},
	}

	err = p.ProcessUnit(u1)
	require.NoError(t, err)

	require.Equal(t, unit.PayloadMPEG4Video{
		0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode),
		0, 0, 1, 0xFF,
		0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode),
		0, 0, 1, 0xF0,
	}, u1.Payload)

	u2 := &unit.Unit{
		PTS: 30000 * 2,
		Payload: unit.PayloadMPEG4Video{
			0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode),
			0, 0, 1, 0xF1,
		},
	}

	err = p.ProcessUnit(u2)
	require.NoError(t, err)

	// test that params have been added to the SDP
	require.Equal(t, []byte{
		0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode),
		0, 0, 1, 0xFF,
	}, forma.Config)

	// test that params have been added to the frame
	require.Equal(t, unit.PayloadMPEG4Video{
		0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode),
		0, 0, 1, 0xFF,
		0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode),
		0, 0, 1, 0xF1,
	}, u2.Payload)

	// test that timestamp has increased
	require.Equal(t, u1.RTPPackets[0].Timestamp+30000, u2.RTPPackets[0].Timestamp)
}
