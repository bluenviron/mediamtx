package formatprocessor

import (
	"testing"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestGenericRemovePadding(t *testing.T) {
	forma := &format.Generic{
		PayloadTyp: 96,
		RTPMap:     "private/90000",
	}
	forma.Init()

	p, err := New(forma, false)
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

	err = p.Process(&DataGeneric{
		RTPPackets: []*rtp.Packet{pkt},
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
