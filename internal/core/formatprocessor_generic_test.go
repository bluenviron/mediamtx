package core

import (
	"testing"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestFormatProcessorRemovePadding(t *testing.T) {
	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"webrtcDisable: yes\n" +
		"paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	forma := &format.Generic{
		PayloadTyp: 96,
		RTPMap:     "private/90000",
	}
	forma.Init()
	medi := &media.Media{
		Type:    media.TypeApplication,
		Formats: []format.Format{forma},
	}
	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://localhost:8554/stream",
		media.Medias{medi})
	require.NoError(t, err)
	defer source.Close()

	c := gortsplib.Client{}

	u, err := url.Parse("rtsp://127.0.0.1:8554/stream")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	medias, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	err = c.SetupAll(medias, baseURL)
	require.NoError(t, err)

	packetRecv := make(chan struct{})

	c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
		require.Equal(t, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
				CSRC:           []uint32{},
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}, pkt)
		close(packetRecv)
	})

	_, err = c.Play(nil)
	require.NoError(t, err)

	source.WritePacketRTP(medi, &rtp.Packet{
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
	})

	<-packetRecv
}
