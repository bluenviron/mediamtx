package srt

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
	srtlib "github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestDest(t *testing.T) {
	ln, err := srtlib.Listen("srt", "127.0.0.1:0", srtlib.DefaultConfig())
	require.NoError(t, err)
	defer ln.Close()

	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	require.NoError(t, subStream.Initialize())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := &Dest{
		Dest:              "srt://" + ln.Addr().String(),
		WriteTimeout:      conf.Duration(10 * time.Second),
		UDPMaxPayloadSize: 1472,
		Parent:            test.NilLogger,
	}

	done := make(chan error, 1)
	go func() {
		done <- dest.Run(ctx, strm)
	}()

	acceptTimer := time.AfterFunc(5*time.Second, ln.Close)
	req, err := ln.Accept2()
	if !acceptTimer.Stop() {
		t.Fatal("timed out waiting for SRT connection")
	}
	require.NoError(t, err)

	conn, err := req.Accept()
	require.NoError(t, err)
	defer conn.Close()
	readTimer := time.AfterFunc(5*time.Second, func() {
		_ = conn.Close()
	})
	defer readTimer.Stop()

	strm.WaitForReaders()
	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:     0,
		Payload: unit.PayloadH264{{5, 1}},
	})

	reader := &mpegts.Reader{R: conn}
	require.NoError(t, reader.Initialize())
	require.Equal(t, []*mpegts.Track{{
		PID:   256,
		Codec: &tscodecs.H264{},
	}}, reader.Tracks())

	received := false
	reader.OnDataH264(reader.Tracks()[0], func(pts int64, dts int64, au [][]byte) error {
		require.Equal(t, int64(0), pts)
		require.Equal(t, int64(0), dts)
		require.Equal(t, [][]byte{
			test.FormatH264.SPS,
			test.FormatH264.PPS,
			{5, 1},
		}, au)
		received = true
		return nil
	})

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:     90000,
		Payload: unit.PayloadH264{{5, 2}},
	})

	for !received {
		require.NoError(t, reader.Read())
	}

	require.Eventually(t, func() bool {
		return dest.OutboundBytes() > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case runErr := <-done:
		require.Error(t, runErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SRT destination to stop")
	}
}
