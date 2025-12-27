package srt

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
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

	p := &test.StaticSourceParent{}
	p.Initialize()

	so := &Source{
		ReadTimeout: conf.Duration(10 * time.Second),
		Parent:      p,
	}

	done := make(chan struct{})
	defer func() { <-done }()

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	reloadConf := make(chan *conf.Path)

	go func() {
		so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
			Context:        ctx,
			ResolvedSource: "srt://127.0.0.1:9002?streamid=sidname&passphrase=ttest1234567",
			Conf:           &conf.Path{},
			ReloadConf:     reloadConf,
		})
		close(done)
	}()

	req, err2 := ln.Accept2()
	require.NoError(t, err2)

	require.Equal(t, "sidname", req.StreamId())
	err2 = req.SetPassphrase("ttest1234567")
	require.NoError(t, err2)

	conn, err2 := req.Accept()
	require.NoError(t, err2)
	defer conn.Close()

	track := &mpegts.Track{Codec: &tscodecs.H264{}}

	w := &mpegts.Writer{W: conn, Tracks: []*mpegts.Track{track}}
	err2 = w.Initialize()
	require.NoError(t, err2)

	err2 = w.WriteH264(track, 0, 0, [][]byte{{ // IDR
		5, 1,
	}})
	require.NoError(t, err2)

	err = w.WriteH264(track, 0, 0, [][]byte{{ // non-IDR
		5, 2,
	}})
	require.NoError(t, err)

	<-p.Unit

	// the source must be listening on ReloadConf
	reloadConf <- nil

	// stop test reader before 2nd H264 packet is received to avoid a crash
	p.Close()
}
