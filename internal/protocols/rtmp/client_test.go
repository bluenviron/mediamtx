package rtmp

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/handshake"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

func TestClient(t *testing.T) {
	for _, ca := range []string{
		"auth",
		"read",
		"read nginx rtmp",
		"publish",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})
			authState := 0

			go func() {
				for {
					conn, err2 := ln.Accept()
					require.NoError(t, err2)
					defer conn.Close()
					bc := bytecounter.NewReadWriter(conn)

					_, _, err2 = handshake.DoServer(bc, false)
					require.NoError(t, err2)

					mrw := message.NewReadWriter(bc, bc, true)

					msg, err2 := mrw.Read()
					require.NoError(t, err2)
					require.Equal(t, &message.SetWindowAckSize{
						Value: 2500000,
					}, msg)

					msg, err2 = mrw.Read()
					require.NoError(t, err2)
					require.Equal(t, &message.SetPeerBandwidth{
						Value: 2500000,
						Type:  2,
					}, msg)

					msg, err2 = mrw.Read()
					require.NoError(t, err2)
					require.Equal(t, &message.SetChunkSize{
						Value: 65536,
					}, msg)

					switch ca {
					case "auth":
						msg, err2 = mrw.Read()
						require.NoError(t, err2)

						switch authState {
						case 0:
							require.Equal(t, &message.CommandAMF0{
								ChunkStreamID: 3,
								Name:          "connect",
								CommandID:     1,
								Arguments: []interface{}{
									amf0.Object{
										{Key: "app", Value: "stream"},
										{Key: "flashVer", Value: "LNX 9,0,124,2"},
										{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream"},
										{Key: "fpad", Value: false},
										{Key: "capabilities", Value: float64(15)},
										{Key: "audioCodecs", Value: float64(4071)},
										{Key: "videoCodecs", Value: float64(252)},
										{Key: "videoFunction", Value: float64(1)},
									},
								},
							}, msg)

						case 1:
							require.Equal(t, &message.CommandAMF0{
								ChunkStreamID: 3,
								Name:          "connect",
								CommandID:     1,
								Arguments: []interface{}{
									amf0.Object{
										{Key: "app", Value: "stream?authmod=adobe&user=myuser"},
										{Key: "flashVer", Value: "LNX 9,0,124,2"},
										{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream?authmod=adobe&user=myuser"},
										{Key: "fpad", Value: false},
										{Key: "capabilities", Value: float64(15)},
										{Key: "audioCodecs", Value: float64(4071)},
										{Key: "videoCodecs", Value: float64(252)},
										{Key: "videoFunction", Value: float64(1)},
									},
								},
							}, msg)

						case 2:
							app, _ := msg.(*message.CommandAMF0).Arguments[0].(amf0.Object).GetString("app")
							query := queryDecode(app[len("stream?"):])
							clientChallenge := query["challenge"]
							response := authResponse("myuser", "mypass", "salt123", "", "server456challenge", clientChallenge)

							require.Equal(t, &message.CommandAMF0{
								ChunkStreamID: 3,
								Name:          "connect",
								CommandID:     1,
								Arguments: []interface{}{
									amf0.Object{
										{
											Key: "app",
											Value: "stream?authmod=adobe&user=myuser&challenge=" +
												clientChallenge + "&response=" + response,
										},
										{Key: "flashVer", Value: "LNX 9,0,124,2"},
										{
											Key: "tcUrl",
											Value: "rtmp://127.0.0.1:9121/stream?authmod=adobe&user=myuser&challenge=" +
												clientChallenge + "&response=" + response,
										},
										{Key: "fpad", Value: false},
										{Key: "capabilities", Value: float64(15)},
										{Key: "audioCodecs", Value: float64(4071)},
										{Key: "videoCodecs", Value: float64(252)},
										{Key: "videoFunction", Value: float64(1)},
									},
								},
							}, msg)
						}

					case "read", "read nginx rtmp":
						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "connect",
							CommandID:     1,
							Arguments: []interface{}{
								amf0.Object{
									{Key: "app", Value: "stream"},
									{Key: "flashVer", Value: "LNX 9,0,124,2"},
									{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream"},
									{Key: "fpad", Value: false},
									{Key: "capabilities", Value: float64(15)},
									{Key: "audioCodecs", Value: float64(4071)},
									{Key: "videoCodecs", Value: float64(252)},
									{Key: "videoFunction", Value: float64(1)},
								},
							},
						}, msg)

					case "publish":
						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "connect",
							CommandID:     1,
							Arguments: []interface{}{
								amf0.Object{
									{Key: "app", Value: "stream"},
									{Key: "flashVer", Value: "LNX 9,0,124,2"},
									{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream"},
								},
							},
						}, msg)
					}

					if ca == "auth" {
						switch authState {
						case 0:
							err2 = mrw.Write(&message.CommandAMF0{
								ChunkStreamID: 3,
								Name:          "_error",
								CommandID:     1,
								Arguments: []interface{}{
									nil,
									amf0.Object{
										{Key: "level", Value: "error"},
										{Key: "code", Value: "NetConnection.Connect.Rejected"},
										{Key: "description", Value: "code=403 need auth; authmod=adobe"},
									},
								},
							})
							require.NoError(t, err2)

							authState++
							continue

						case 1:
							err2 = mrw.Write(&message.CommandAMF0{
								ChunkStreamID: 3,
								Name:          "_error",
								CommandID:     1,
								Arguments: []interface{}{
									nil,
									amf0.Object{
										{Key: "level", Value: "error"},
										{Key: "code", Value: "NetConnection.Connect.Rejected"},
										{
											Key:   "description",
											Value: "authmod=adobe ?reason=needauth&user=myuser&salt=salt123&challenge=server456challenge",
										},
									},
								},
							})
							require.NoError(t, err2)

							authState++
							continue
						}
					}

					err2 = mrw.Write(&message.CommandAMF0{
						ChunkStreamID: 3,
						Name:          "_result",
						CommandID:     1,
						Arguments: []interface{}{
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
					require.NoError(t, err2)

					switch ca {
					case "auth", "read", "read nginx rtmp":
						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "createStream",
							CommandID:     2,
							Arguments: []interface{}{
								nil,
							},
						}, msg)

						err2 = mrw.Write(&message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "_result",
							CommandID:     2,
							Arguments: []interface{}{
								nil,
								float64(1),
							},
						})
						require.NoError(t, err2)

						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.UserControlSetBufferLength{
							BufferLength: 0x64,
						}, msg)

						msg, err2 = mrw.Read()
						require.NoError(t, err2)
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

						err2 = mrw.Write(&message.CommandAMF0{
							ChunkStreamID:   5,
							MessageStreamID: 0x1000000,
							Name:            "onStatus",
							CommandID: func() int {
								if ca == "read nginx rtmp" {
									return 0
								}
								return 3
							}(),
							Arguments: []interface{}{
								nil,
								amf0.Object{
									{Key: "level", Value: "status"},
									{Key: "code", Value: "NetStream.Play.Reset"},
									{Key: "description", Value: "play reset"},
								},
							},
						})
						require.NoError(t, err2)

					case "publish":
						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "releaseStream",
							CommandID:     2,
							Arguments: []interface{}{
								nil,
								"",
							},
						}, msg)

						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "FCPublish",
							CommandID:     3,
							Arguments: []interface{}{
								nil,
								"",
							},
						}, msg)

						msg, err2 = mrw.Read()
						require.NoError(t, err2)
						require.Equal(t, &message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "createStream",
							CommandID:     4,
							Arguments: []interface{}{
								nil,
							},
						}, msg)

						err2 = mrw.Write(&message.CommandAMF0{
							ChunkStreamID: 3,
							Name:          "_result",
							CommandID:     4,
							Arguments: []interface{}{
								nil,
								float64(1),
							},
						})
						require.NoError(t, err2)

						msg, err2 = mrw.Read()
						require.NoError(t, err2)
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

						err2 = mrw.Write(&message.CommandAMF0{
							ChunkStreamID:   5,
							MessageStreamID: 0x1000000,
							Name:            "onStatus",
							CommandID:       5,
							Arguments: []interface{}{
								nil,
								amf0.Object{
									{Key: "level", Value: "status"},
									{Key: "code", Value: "NetStream.Publish.Start"},
									{Key: "description", Value: "publish start"},
								},
							},
						})
						require.NoError(t, err2)
					}

					close(done)
					break
				}
			}()

			var rawURL string

			if ca == "auth" {
				rawURL = "rtmp://myuser:mypass@127.0.0.1:9121/stream"
			} else {
				rawURL = "rtmp://127.0.0.1:9121/stream"
			}

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			conn := &Client{
				URL:     u,
				Publish: (ca == "publish"),
			}
			err = conn.Initialize(context.Background())
			require.NoError(t, err)
			defer conn.Close()

			switch ca {
			case "read", "read nginx rtmp":
				require.Equal(t, uint64(3421), conn.BytesReceived())
				require.Equal(t, uint64(3409), conn.BytesSent())

			case "publish":
				require.Equal(t, uint64(3427), conn.BytesReceived())
				require.Equal(t, uint64(0xd27), conn.BytesSent())
			}

			<-done
		})
	}
}
