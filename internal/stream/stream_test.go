package stream

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
	}}

	strm := &Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               desc,
		GenerateRTPPackets: true,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	r := &Reader{}

	recv := make(chan struct{})

	r.OnData(desc.Medias[0], desc.Medias[0].Formats[0], func(_ *unit.Unit) error {
		close(recv)
		return nil
	})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 30000 * 2,
		Payload: unit.PayloadH264{
			{5, 2}, // IDR
		},
	})

	<-recv

	require.Equal(t, uint64(14), strm.BytesReceived())
	require.Equal(t, uint64(14), strm.BytesSent())
}

func TestStreamSkipBytesSent(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
	}}

	strm := &Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               desc,
		GenerateRTPPackets: true,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	r := &Reader{
		SkipBytesSent: true,
	}

	recv := make(chan struct{})

	r.OnData(desc.Medias[0], desc.Medias[0].Formats[0], func(_ *unit.Unit) error {
		close(recv)
		return nil
	})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 30000 * 2,
		Payload: unit.PayloadH264{
			{5, 2}, // IDR
		},
	})

	<-recv

	require.Equal(t, uint64(14), strm.BytesReceived())
	require.Equal(t, uint64(0), strm.BytesSent())
}
