package formatprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestLPCMProcessUnit(t *testing.T) {
	forma := &format.LPCM{
		PayloadTyp:   96,
		BitDepth:     16,
		SampleRate:   44100,
		ChannelCount: 2,
	}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	unit := &unit.LPCM{
		Samples: []byte{1, 2, 3, 4},
	}

	err = p.ProcessUnit(unit)
	require.NoError(t, err)
	require.Equal(t, []*rtp.Packet{{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: unit.RTPPackets[0].SequenceNumber,
			Timestamp:      unit.RTPPackets[0].Timestamp,
			SSRC:           unit.RTPPackets[0].SSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}}, unit.RTPPackets)
}
