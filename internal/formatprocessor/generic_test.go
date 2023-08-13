package formatprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestGenericRemovePadding(t *testing.T) {
	forma := &formats.Generic{
		PayloadTyp: 96,
		RTPMa:      "private/90000",
	}
	err := forma.Init()
	require.NoError(t, err)

	p, err := New(1472, forma, false, nil)
	require.NoError(t, err)

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
			Padding:        true,
		},
		Payload:     []byte{0x01, 0x02, 0x03, 0x04},
		PaddingSize: 20,
	}

	err = p.Process(&UnitGeneric{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
		},
	}, false)
	require.NoError(t, err)

	require.Equal(t, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}, pkt)
}
