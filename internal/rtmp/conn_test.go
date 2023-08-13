package rtmp

import (
	"bytes"
	"net"
	"net/url"
	"testing"

	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/handshake"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

func TestNewClientConn(t *testing.T) {
	for _, ca := range []string{"read", "publish"} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				conn, err := ln.Accept()
				require.NoError(t, err)
				defer conn.Close()
				bc := bytecounter.NewReadWriter(conn)

				_, _, err = handshake.DoServer(bc, false)
				require.NoError(t, err)

				mrw := message.NewReadWriter(bc, bc, true)

				msg, err := mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.SetWindowAckSize{
					Value: 2500000,
				}, msg)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.SetPeerBandwidth{
					Value: 2500000,
					Type:  2,
				}, msg)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.SetChunkSize{
					Value: 65536,
				}, msg)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "connect",
					CommandID:     1,
					Arguments: []interface{}{
						flvio.AMFMap{
							{K: "app", V: "stream"},
							{K: "flashVer", V: "LNX 9,0,124,2"},
							{K: "tcUrl", V: "rtmp://127.0.0.1:9121/stream"},
							{K: "fpad", V: false},
							{K: "capabilities", V: float64(15)},
							{K: "audioCodecs", V: float64(4071)},
							{K: "videoCodecs", V: float64(252)},
							{K: "videoFunction", V: float64(1)},
						},
					},
				}, msg)

				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "_result",
					CommandID:     1,
					Arguments: []interface{}{
						flvio.AMFMap{
							{K: "fmsVer", V: "LNX 9,0,124,2"},
							{K: "capabilities", V: float64(31)},
						},
						flvio.AMFMap{
							{K: "level", V: "status"},
							{K: "code", V: "NetConnection.Connect.Success"},
							{K: "description", V: "Connection succeeded."},
							{K: "objectEncoding", V: float64(0)},
						},
					},
				})
				require.NoError(t, err)

				if ca == "read" {
					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "createStream",
						CommandID:     2,
						Arguments: []interface{}{
							nil,
						},
					}, msg)

					err = mrw.Write(&message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "_result",
						CommandID:     2,
						Arguments: []interface{}{
							nil,
							float64(1),
						},
					})
					require.NoError(t, err)

					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.UserControlSetBufferLength{
						BufferLength: 0x64,
					}, msg)

					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.CommandAMF0{
						ChunkStreamID:   4,
						MessageStreamID: 0x1000000,
						Name:            "play",
						CommandID:       3,
						Arguments: []interface{}{
							nil,
							"",
						},
					}, msg)

					err = mrw.Write(&message.CommandAMF0{
						ChunkStreamID:   5,
						MessageStreamID: 0x1000000,
						Name:            "onStatus",
						CommandID:       3,
						Arguments: []interface{}{
							nil,
							flvio.AMFMap{
								{K: "level", V: "status"},
								{K: "code", V: "NetStream.Play.Reset"},
								{K: "description", V: "play reset"},
							},
						},
					})
					require.NoError(t, err)
				} else {
					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "releaseStream",
						CommandID:     2,
						Arguments: []interface{}{
							nil,
							"",
						},
					}, msg)

					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "FCPublish",
						CommandID:     3,
						Arguments: []interface{}{
							nil,
							"",
						},
					}, msg)

					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "createStream",
						CommandID:     4,
						Arguments: []interface{}{
							nil,
						},
					}, msg)

					err = mrw.Write(&message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "_result",
						CommandID:     4,
						Arguments: []interface{}{
							nil,
							float64(1),
						},
					})
					require.NoError(t, err)

					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.CommandAMF0{
						ChunkStreamID:   4,
						MessageStreamID: 0x1000000,
						Name:            "publish",
						CommandID:       5,
						Arguments: []interface{}{
							nil,
							"",
							"stream",
						},
					}, msg)

					err = mrw.Write(&message.CommandAMF0{
						ChunkStreamID:   5,
						MessageStreamID: 0x1000000,
						Name:            "onStatus",
						CommandID:       5,
						Arguments: []interface{}{
							nil,
							flvio.AMFMap{
								{K: "level", V: "status"},
								{K: "code", V: "NetStream.Publish.Start"},
								{K: "description", V: "publish start"},
							},
						},
					})
					require.NoError(t, err)
				}

				close(done)
			}()

			u, err := url.Parse("rtmp://127.0.0.1:9121/stream")
			require.NoError(t, err)

			nconn, err := net.Dial("tcp", u.Host)
			require.NoError(t, err)
			defer nconn.Close()

			conn, err := NewClientConn(nconn, u, ca == "publish")
			require.NoError(t, err)

			if ca == "read" {
				require.Equal(t, uint64(3421), conn.BytesReceived())
				require.Equal(t, uint64(3409), conn.BytesSent())
			} else {
				require.Equal(t, uint64(3427), conn.BytesReceived())
				require.Equal(t, uint64(3466), conn.BytesSent())
			}

			<-done
		})
	}
}

func TestNewServerConn(t *testing.T) {
	for _, ca := range []string{
		"read",
		"publish",
		"publish neko",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				nconn, err := ln.Accept()
				require.NoError(t, err)
				defer nconn.Close()

				_, u, isPublishing, err := NewServerConn(nconn)
				require.NoError(t, err)

				require.Equal(t, &url.URL{
					Scheme: "rtmp",
					Host:   "127.0.0.1:9121",
					Path:   "//stream/",
				}, u)
				require.Equal(t, ca == "publish" || ca == "publish neko", isPublishing)

				close(done)
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()
			bc := bytecounter.NewReadWriter(conn)

			_, _, err = handshake.DoClient(bc, false, false)
			require.NoError(t, err)

			mrw := message.NewReadWriter(bc, bc, true)

			tcURL := "rtmp://127.0.0.1:9121/stream"
			if ca == "publish neko" {
				tcURL = "'rtmp://127.0.0.1:9121/stream"
			}

			err = mrw.Write(&message.CommandAMF0{
				ChunkStreamID: 3,
				Name:          "connect",
				CommandID:     1,
				Arguments: []interface{}{
					flvio.AMFMap{
						{K: "app", V: "/stream"},
						{K: "flashVer", V: "LNX 9,0,124,2"},
						{K: "tcUrl", V: tcURL},
						{K: "fpad", V: false},
						{K: "capabilities", V: 15},
						{K: "audioCodecs", V: 4071},
						{K: "videoCodecs", V: 252},
						{K: "videoFunction", V: 1},
					},
				},
			})
			require.NoError(t, err)

			msg, err := mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.SetWindowAckSize{
				Value: 2500000,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.SetPeerBandwidth{
				Value: 2500000,
				Type:  2,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.SetChunkSize{
				Value: 65536,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.CommandAMF0{
				ChunkStreamID: 3,
				Name:          "_result",
				CommandID:     1,
				Arguments: []interface{}{
					flvio.AMFMap{
						{K: "fmsVer", V: "LNX 9,0,124,2"},
						{K: "capabilities", V: float64(31)},
					},
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetConnection.Connect.Success"},
						{K: "description", V: "Connection succeeded."},
						{K: "objectEncoding", V: float64(0)},
					},
				},
			}, msg)

			err = mrw.Write(&message.SetChunkSize{
				Value: 65536,
			})
			require.NoError(t, err)

			if ca == "read" {
				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "createStream",
					CommandID:     2,
					Arguments: []interface{}{
						nil,
					},
				})
				require.NoError(t, err)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "_result",
					CommandID:     2,
					Arguments: []interface{}{
						nil,
						float64(1),
					},
				}, msg)

				err = mrw.Write(&message.UserControlSetBufferLength{
					BufferLength: 0x64,
				})
				require.NoError(t, err)

				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					Name:            "play",
					CommandID:       0,
					Arguments: []interface{}{
						nil,
						"",
					},
				})
				require.NoError(t, err)
			} else {
				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "releaseStream",
					CommandID:     2,
					Arguments: []interface{}{
						nil,
						"",
					},
				})
				require.NoError(t, err)

				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "FCPublish",
					CommandID:     3,
					Arguments: []interface{}{
						nil,
						"",
					},
				})
				require.NoError(t, err)

				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "createStream",
					CommandID:     4,
					Arguments: []interface{}{
						nil,
					},
				})
				require.NoError(t, err)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "_result",
					CommandID:     4,
					Arguments: []interface{}{
						nil,
						float64(1),
					},
				}, msg)

				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					Name:            "publish",
					CommandID:       5,
					Arguments: []interface{}{
						nil,
						"",
						"stream",
					},
				})
				require.NoError(t, err)
			}

			<-done
		})
	}
}

func BenchmarkRead(b *testing.B) {
	var buf bytes.Buffer

	for n := 0; n < b.N; n++ {
		buf.Write([]byte{
			7, 0, 0, 23, 0, 0, 98, 8,
			0, 0, 0, 64, 175, 1, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4, 1, 2,
			3, 4, 1, 2, 3, 4,
		})
	}

	conn := newNoHandshakeConn(&buf)

	for n := 0; n < b.N; n++ {
		conn.Read() //nolint:errcheck
	}
}
