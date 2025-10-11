package codecprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestGenericProcessRTPPacket(t *testing.T) {
	forma := &format.Generic{
		PayloadTyp: 96,
		RTPMa:      "private/90000",
	}
	err := forma.Init()
	require.NoError(t, err)

	p, err := New(1450, forma, false, nil)
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
		Payload:     []byte{1, 2, 3, 4},
		PaddingSize: 20,
	}

	u := &unit.Unit{RTPPackets: []*rtp.Packet{pkt}}
	err = p.ProcessRTPPacket(u, false)
	require.NoError(t, err)

	// check that padding has been removed
	require.Equal(t, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{1, 2, 3, 4},
	}, pkt)
}
