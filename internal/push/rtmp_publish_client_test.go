package push

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/bytecounter"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/stretchr/testify/require"
)

func TestRTMPPublishClientPublishType(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	serverDone := make(chan struct{})
	defer func() { <-serverDone }()

	go func() {
		defer close(serverDone)

		nconn, acceptErr := ln.Accept()
		require.NoError(t, acceptErr)
		defer nconn.Close()

		bc := bytecounter.NewReadWriter(nconn)

		_, _, handshakeErr := handshake.DoServer(bc, false)
		require.NoError(t, handshakeErr)

		mrw := message.NewReadWriter(bc, bc, true)

		for {
			msg, readErr := mrw.Read()
			require.NoError(t, readErr)

			if msg, ok := msg.(*message.CommandAMF0); ok && msg.Name == "connect" {
				break
			}
		}

		writeErr := mrw.Write(&message.CommandAMF0{
			ChunkStreamID: 3,
			Name:          "_result",
			CommandID:     1,
			Arguments: []any{
				amf0.Object{
					{Key: "fmsVer", Value: "LNX 9,0,124,2"},
					{Key: "capabilities", Value: float64(31)},
				},
				amf0.Object{
					{Key: "level", Value: "status"},
					{Key: "code", Value: "NetConnection.Connect.Success"},
					{Key: "description", Value: "Connection succeeded."},
					{Key: "objectEncoding", Value: float64(0)},
				},
			},
		})
		require.NoError(t, writeErr)

		for {
			msg, readErr := mrw.Read()
			require.NoError(t, readErr)

			if msg, ok := msg.(*message.CommandAMF0); ok && msg.Name == "createStream" {
				break
			}
		}

		writeErr = mrw.Write(&message.CommandAMF0{
			ChunkStreamID: 3,
			Name:          "_result",
			CommandID:     4,
			Arguments: []any{
				nil,
				float64(1),
			},
		})
		require.NoError(t, writeErr)

		for {
			msg, readErr := mrw.Read()
			require.NoError(t, readErr)

			cmd, ok := msg.(*message.CommandAMF0)
			if !ok || cmd.Name != "publish" {
				continue
			}

			require.Len(t, cmd.Arguments, 3)
			require.Nil(t, cmd.Arguments[0])
			require.Equal(t, "stream", cmd.Arguments[1])
			require.Equal(t, rtmpPublishTypeLive, cmd.Arguments[2])
			break
		}

		writeErr = mrw.Write(&message.CommandAMF0{
			ChunkStreamID:   5,
			MessageStreamID: 0x1000000,
			Name:            "onStatus",
			CommandID:       5,
			Arguments: []any{
				nil,
				amf0.Object{
					{Key: "level", Value: "status"},
					{Key: "code", Value: "NetStream.Publish.Start"},
					{Key: "description", Value: "publish start"},
				},
			},
		})
		require.NoError(t, writeErr)
	}()

	u, err := url.Parse("rtmp://" + ln.Addr().String() + "/app/stream")
	require.NoError(t, err)

	c := &rtmpPublishClient{URL: u}
	require.NoError(t, c.Initialize(context.Background()))
	defer c.Close()
}
