package rtmp

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	rtmpcodecs "github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestDest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	received := make(chan [][]byte, 8)
	serverErr := make(chan error, 1)
	go func() {
		nconn, listenAcceptErr := ln.Accept()
		if listenAcceptErr != nil {
			serverErr <- listenAcceptErr
			return
		}
		defer nconn.Close()

		conn := &gortmplib.ServerConn{RW: nconn}
		if initializeErr := conn.Initialize(); initializeErr != nil {
			serverErr <- initializeErr
			return
		}
		if publishAcceptErr := conn.Accept(); publishAcceptErr != nil {
			serverErr <- publishAcceptErr
			return
		}
		if !conn.Publish || conn.URL.Path != "/stream" {
			serverErr <- fmt.Errorf("unexpected publish target: %s", conn.URL)
			return
		}

		reader := &gortmplib.Reader{Conn: conn}
		if initializeErr := reader.Initialize(); initializeErr != nil {
			serverErr <- initializeErr
			return
		}

		tracks := reader.Tracks()
		if len(tracks) != 1 {
			serverErr <- fmt.Errorf("unexpected track count: %d", len(tracks))
			return
		}
		if _, ok := tracks[0].Codec.(*rtmpcodecs.H264); !ok {
			serverErr <- fmt.Errorf("unexpected codec: %T", tracks[0].Codec)
			return
		}

		reader.OnDataH264(tracks[0], func(_ time.Duration, _ time.Duration, au [][]byte) {
			select {
			case received <- au:
			default:
			}
		})

		for {
			if readErr := reader.Read(); readErr != nil {
				return
			}
		}
	}()

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

	u, err := url.Parse("rtmp://" + ln.Addr().String() + "/stream")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := &Dest{
		URL:          u,
		WriteTimeout: conf.Duration(10 * time.Second),
		Parent:       test.NilLogger,
	}

	done := make(chan error, 1)
	go func() {
		done <- dest.Run(ctx, strm)
	}()

	strm.WaitForReaders()
	for i := range 2 {
		subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS:     int64(i) * 2 * 90000,
			Payload: unit.PayloadH264{{5, 2}},
		})
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

frameLoop:
	for {
		select {
		case au := <-received:
			for _, nalu := range au {
				if bytes.Equal(nalu, []byte{5, 2}) {
					break frameLoop
				}
			}
		case receivedErr := <-serverErr:
			require.NoError(t, receivedErr)
		case runErr := <-done:
			t.Fatalf("RTMP destination stopped before forwarding a frame: %v", runErr)
		case <-timer.C:
			t.Fatal("timed out waiting for RTMP frame")
		}
	}

	require.Eventually(t, func() bool {
		return dest.OutboundBytes() > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case runErr := <-done:
		require.Error(t, runErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTMP destination to stop")
	}
}

func TestFourCCList(t *testing.T) {
	require.Empty(t, fourCCList(&description.Session{Medias: []*description.Media{{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp: 96,
		}},
	}}}))

	require.NotEmpty(t, fourCCList(&description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.H265{}},
	}}}))
}
