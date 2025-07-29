package srt

import (
	"bufio"
	"context"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestSource(t *testing.T) {
	ln, err := srt.Listen("srt", "127.0.0.1:9002", srt.DefaultConfig())
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		req, err2 := ln.Accept2()
		require.NoError(t, err2)

		require.Equal(t, "sidname", req.StreamId())
		err2 = req.SetPassphrase("ttest1234567")
		require.NoError(t, err2)

		conn, err2 := req.Accept()
		require.NoError(t, err2)
		defer conn.Close()

		track := &mpegts.Track{
			Codec: &mpegts.CodecH264{},
		}

		bw := bufio.NewWriter(conn)
		w := &mpegts.Writer{W: bw, Tracks: []*mpegts.Track{track}}
		err2 = w.Initialize()
		require.NoError(t, err2)

		err2 = w.WriteH264(track, 0, 0, [][]byte{{ // IDR
			5, 1,
		}})
		require.NoError(t, err2)

		err2 = bw.Flush()
		require.NoError(t, err2)

		// wait for internal SRT queue to be written
		time.Sleep(500 * time.Millisecond)
	}()

	p := &test.StaticSourceParent{}
	p.Initialize()
	defer p.Close()

	so := &Source{
		ReadTimeout: conf.Duration(10 * time.Second),
		Parent:      p,
	}

	done := make(chan struct{})
	defer func() { <-done }()

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	go func() {
		so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
			Context:        ctx,
			ResolvedSource: "srt://127.0.0.1:9002?streamid=sidname&passphrase=ttest1234567",
			Conf:           &conf.Path{},
		})
		close(done)
	}()

	<-p.Unit
}
