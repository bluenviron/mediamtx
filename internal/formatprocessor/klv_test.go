package formatprocessor

import (
	"fmt"
	"testing"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestKlvCreateEncoder(t *testing.T) {
	// create KLV format processor
	klv_codec := mpegts.CodecKLV{
		StreamType:      astits.StreamTypePrivateData,
		StreamID:        1,
		PTSDTSIndicator: 1,
	}
	forma := &format.KLV{
		PayloadTyp: 96,
		KLVCodec:   &klv_codec,
	}
	p, err := New(1472, forma, false, nil)
	require.NoError(t, err)
	klvProc := p.(*klv)
	err = klvProc.createEncoder()
	require.NoError(t, err)
}

func TestKlvProcessUnit(t *testing.T) {
	// create KLV format processor
	klv_codec := mpegts.CodecKLV{
		StreamType:      astits.StreamTypePrivateData,
		StreamID:        1,
		PTSDTSIndicator: 1,
	}
	forma := &format.KLV{
		PayloadTyp: 96,
		KLVCodec:   &klv_codec,
	}
	p, err := New(1472, forma, true, nil)
	require.NoError(t, err)
	// create test Unit
	theTime := time.Now()
	when := int64(5000000000) // 5 seconds in nanoseconds
	u := unit.KLV{
		unit.Base{
			nil,
			theTime,
			when,
		},
		[]byte{1, 2, 3, 4},
	}
	uu := &u
	// process the unit
	err = p.ProcessUnit(uu)
	require.NoError(t, err)
	// compare output
	for i, pkt := range u.RTPPackets {
		fmt.Printf("RTP packet[%v]: %+v\n", i, pkt)
	}
}

func TestKlvProcessRTPPacket(t *testing.T) {
	// create KLV format processor
	klv_codec := mpegts.CodecKLV{
		StreamType:      astits.StreamTypePrivateData,
		StreamID:        1,
		PTSDTSIndicator: 1,
	}
	forma := &format.KLV{
		PayloadTyp: 96,
		KLVCodec:   &klv_codec,
	}
	p, err := New(1472, forma, false, nil)
	require.NoError(t, err)
	// create test RTP packet
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
	// process the RTP packet
	_, err = p.ProcessRTPPacket(pkt, time.Time{}, 0, false)
	require.NoError(t, err)
	// compare output
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
