package formatprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestG611Encode(t *testing.T) {
	t.Run("alaw", func(t *testing.T) {
		forma := &format.G711{
			PayloadTyp:   8,
			MULaw:        false,
			SampleRate:   8000,
			ChannelCount: 1,
		}

		p, err := New(1472, forma, true)
		require.NoError(t, err)

		unit := &unit.G711{
			Samples: []byte{1, 2, 3, 4},
		}

		err = p.ProcessUnit(unit)
		require.NoError(t, err)
		require.Equal(t, []*rtp.Packet{{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    8,
				SequenceNumber: unit.RTPPackets[0].SequenceNumber,
				Timestamp:      unit.RTPPackets[0].Timestamp,
				SSRC:           unit.RTPPackets[0].SSRC,
			},
			Payload: []byte{1, 2, 3, 4},
		}}, unit.RTPPackets)
	})

	t.Run("mulaw", func(t *testing.T) {
		forma := &format.G711{
			PayloadTyp:   0,
			MULaw:        true,
			SampleRate:   8000,
			ChannelCount: 1,
		}

		p, err := New(1472, forma, true)
		require.NoError(t, err)

		unit := &unit.G711{
			Samples: []byte{1, 2, 3, 4},
		}

		err = p.ProcessUnit(unit)
		require.NoError(t, err)
		require.Equal(t, []*rtp.Packet{{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: unit.RTPPackets[0].SequenceNumber,
				Timestamp:      unit.RTPPackets[0].Timestamp,
				SSRC:           unit.RTPPackets[0].SSRC,
			},
			Payload: []byte{1, 2, 3, 4},
		}}, unit.RTPPackets)
	})
}
