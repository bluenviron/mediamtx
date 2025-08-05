package formatprocessor

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestKlvCreateEncoder(t *testing.T) {
	forma := &format.KLV{
		PayloadTyp: 96,
	}
	p, err := New(1472, forma, false, nil)
	require.NoError(t, err)

	klvProc := p.(*klv)
	err = klvProc.createEncoder()
	require.NoError(t, err)
}

func TestKlvProcessUnit(t *testing.T) {
	forma := &format.KLV{
		PayloadTyp: 96,
	}
	p, err := New(1472, forma, true, nil)
	require.NoError(t, err)

	// create test Unit
	theTime := time.Now()
	when := int64(5000000000) // 5 seconds in nanoseconds
	u := unit.KLV{
		Base: unit.Base{
			RTPPackets: nil,
			NTP:        theTime,
			PTS:        when,
		},
		Unit: []byte{1, 2, 3, 4},
	}
	uu := &u

	// process the unit
	err = p.ProcessUnit(uu)
	require.NoError(t, err)
}

func TestKlvProcessRTPPacket(t *testing.T) {
	forma := &format.KLV{
		PayloadTyp: 96,
	}
	p, err := New(1472, forma, false, nil)
	require.NoError(t, err)

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 3446,
			Timestamp:      175349,
			SSRC:           563423,
			Padding:        true,
		},
		Payload:     []byte{1, 2, 3, 4},
		PaddingSize: 20,
	}
	_, err = p.ProcessRTPPacket(pkt, time.Time{}, 0, false)
	require.NoError(t, err)

	require.Equal(t, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 3446,
			Timestamp:      175349,
			SSRC:           563423,
		},
		Payload: []byte{1, 2, 3, 4},
	}, pkt)
}
