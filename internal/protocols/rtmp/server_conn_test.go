package rtmp

import (
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/handshake"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestServerConn(t *testing.T) {
	for _, ca := range []string{
		"auth 1",
		"auth 2",
		"auth 3",
		"read",
		"publish",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				defer close(done)

				nconn, err2 := ln.Accept()
				require.NoError(t, err2)
				defer nconn.Close()

				conn := &ServerConn{
					RW: nconn,
				}
				err2 = conn.Initialize()
				require.NoError(t, err2)

				if ca == "auth 1" || ca == "auth 2" || ca == "auth 3" {
					err2 = conn.CheckCredentials("myuser", "mypass")
					switch ca {
					case "auth 1":
						require.Error(t, err2, "need auth")
						return
					case "auth 2":
						require.Error(t, err2, "need auth 2")
						return
					case "auth 3":
						require.NoError(t, err2)
					}
				}

				err2 = conn.Accept()
				require.NoError(t, err2)

				require.Equal(t, &url.URL{
					Scheme:   "rtmp",
					Host:     "127.0.0.1:9121",
					Path:     "/stream",
					RawQuery: "key=val",
				}, conn.URL)
				require.Equal(t, (ca == "publish"), conn.Publish)
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()
			bc := bytecounter.NewReadWriter(conn)

			_, _, err = handshake.DoClient(bc, false, false)
			require.NoError(t, err)

			mrw := message.NewReadWriter(bc, bc, true)

			switch ca {
			case "auth 1": //nolint:dupl
				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "connect",
					CommandID:     1,
					Arguments: []interface{}{
						amf0.Object{
							{Key: "app", Value: "stream?key=val"},
							{Key: "flashVer", Value: "LNX 9,0,124,2"},
							{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream?key=val"},
							{Key: "fpad", Value: false},
							{Key: "capabilities", Value: float64(15)},
							{Key: "audioCodecs", Value: float64(4071)},
							{Key: "videoCodecs", Value: float64(252)},
							{Key: "videoFunction", Value: float64(1)},
						},
					},
				})
				require.NoError(t, err)

				var msg message.Message
				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.CommandAMF0{
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
				}, msg)

			case "auth 2": //nolint:dupl
				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "connect",
					CommandID:     1,
					Arguments: []interface{}{
						amf0.Object{
							{Key: "app", Value: "stream?key=val?authmod=adobe&user=myuser"},
							{Key: "flashVer", Value: "LNX 9,0,124,2"},
							{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream?key=val?authmod=adobe&user=myuser"},
							{Key: "fpad", Value: false},
							{Key: "capabilities", Value: float64(15)},
							{Key: "audioCodecs", Value: float64(4071)},
							{Key: "videoCodecs", Value: float64(252)},
							{Key: "videoFunction", Value: float64(1)},
						},
					},
				})
				require.NoError(t, err)

				var msg message.Message
				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "_error",
					CommandID:     1,
					Arguments: []interface{}{
						nil,
						amf0.Object{
							{Key: "level", Value: "error"},
							{Key: "code", Value: "NetConnection.Connect.Rejected"},
							{Key: "description", Value: "authmod=adobe ?reason=needauth&user=myuser&salt=testsalt&challenge=testchallenge"},
						},
					},
				}, msg)

			case "auth 3":
				clientChallenge := uuid.New().String()
				response := authResponse("myuser", "mypass", serverSalt, "", serverChallenge, clientChallenge)

				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "connect",
					CommandID:     1,
					Arguments: []interface{}{
						amf0.Object{
							{
								Key: "app",
								Value: fmt.Sprintf("stream?key=val?authmod=adobe&user=myuser&challenge=%s&response=%s",
									clientChallenge, response),
							},
							{Key: "flashVer", Value: "LNX 9,0,124,2"},
							{
								Key: "tcUrl",
								Value: fmt.Sprintf("rtmp://127.0.0.1:9121/stream?key=val?authmod=adobe&user=myuser&challenge=%s&response=%s",
									clientChallenge, response),
							},
							{Key: "fpad", Value: false},
							{Key: "capabilities", Value: float64(15)},
							{Key: "audioCodecs", Value: float64(4071)},
							{Key: "videoCodecs", Value: float64(252)},
							{Key: "videoFunction", Value: float64(1)},
						},
					},
				})
				require.NoError(t, err)

				var msg message.Message
				msg, err = mrw.Read()
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
				}, msg)

				err = mrw.Write(&message.SetChunkSize{
					Value: 65536,
				})
				require.NoError(t, err)

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

			case "read":
				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "connect",
					CommandID:     1,
					Arguments: []interface{}{
						amf0.Object{
							{Key: "app", Value: "stream?key=val"},
							{Key: "flashVer", Value: "LNX 9,0,124,2"},
							{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream?key=val"},
							{Key: "fpad", Value: false},
							{Key: "capabilities", Value: float64(15)},
							{Key: "audioCodecs", Value: float64(4071)},
							{Key: "videoCodecs", Value: float64(252)},
							{Key: "videoFunction", Value: float64(1)},
						},
					},
				})
				require.NoError(t, err)

				var msg message.Message
				msg, err = mrw.Read()
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
				}, msg)

				err = mrw.Write(&message.SetChunkSize{
					Value: 65536,
				})
				require.NoError(t, err)

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

			case "publish":
				err = mrw.Write(&message.CommandAMF0{
					ChunkStreamID: 3,
					Name:          "connect",
					CommandID:     1,
					Arguments: []interface{}{
						amf0.Object{
							{Key: "app", Value: "stream?key=val"},
							{Key: "flashVer", Value: "LNX 9,0,124,2"},
							{Key: "tcUrl", Value: "rtmp://127.0.0.1:9121/stream?key=val"},
							{Key: "fpad", Value: false},
							{Key: "capabilities", Value: float64(15)},
							{Key: "audioCodecs", Value: float64(4071)},
							{Key: "videoCodecs", Value: float64(252)},
							{Key: "videoFunction", Value: float64(1)},
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
				}, msg)

				err = mrw.Write(&message.SetChunkSize{
					Value: 65536,
				})
				require.NoError(t, err)

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

func TestServerConnPath(t *testing.T) {
	for _, ca := range []string{
		"standard",
		"leading slash",
		"query",
		"stream key",
		"stream key and query",
		"neko",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				defer close(done)

				nconn, err2 := ln.Accept()
				require.NoError(t, err2)
				defer nconn.Close()

				conn := &ServerConn{
					RW: nconn,
				}
				err2 = conn.Initialize()
				require.NoError(t, err2)

				err2 = conn.Accept()
				require.NoError(t, err2)

				switch ca {
				case "standard", "neko":
					require.Equal(t, &url.URL{
						Scheme: "rtmp",
						Host:   "127.0.0.1:9121",
						Path:   "/stream",
					}, conn.URL)

				case "leading slash":
					require.Equal(t, &url.URL{
						Scheme: "rtmp",
						Host:   "127.0.0.1:9121",
						Path:   "//stream",
					}, conn.URL)

				case "query":
					require.Equal(t, &url.URL{
						Scheme:   "rtmp",
						Host:     "127.0.0.1:9121",
						Path:     "/stream",
						RawQuery: "key=val",
					}, conn.URL)

				case "stream key":
					require.Equal(t, &url.URL{
						Scheme: "rtmp",
						Host:   "127.0.0.1:9121",
						Path:   "/stream/key",
					}, conn.URL)

				case "stream key and query":
					require.Equal(t, &url.URL{
						Scheme:   "rtmp",
						Host:     "127.0.0.1:9121",
						Path:     "/stream/key",
						RawQuery: "key=val",
					}, conn.URL)
				}
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()
			bc := bytecounter.NewReadWriter(conn)

			_, _, err = handshake.DoClient(bc, false, false)
			require.NoError(t, err)

			mrw := message.NewReadWriter(bc, bc, true)

			var app string
			var tcURL string

			switch ca {
			case "standard":
				app = "stream"
				tcURL = "rtmp://127.0.0.1:9121/stream"

			case "leading slash":
				app = "/stream"
				tcURL = "rtmp://127.0.0.1:9121//stream"

			case "query":
				app = "stream?key=val"
				tcURL = "rtmp://127.0.0.1:9121/stream?key=val"

			case "stream key":
				app = "stream"
				tcURL = "rtmp://127.0.0.1:9121/stream"

			case "stream key and query":
				app = "stream"
				tcURL = "rtmp://127.0.0.1:9121/stream"

			case "neko":
				app = "stream"
				tcURL = "'rtmp://127.0.0.1:9121/stream"
			}

			err = mrw.Write(&message.CommandAMF0{
				ChunkStreamID: 3,
				Name:          "connect",
				CommandID:     1,
				Arguments: []interface{}{
					amf0.Object{
						{Key: "app", Value: app},
						{Key: "flashVer", Value: "LNX 9,0,124,2"},
						{Key: "tcUrl", Value: tcURL},
						{Key: "fpad", Value: false},
						{Key: "capabilities", Value: float64(15)},
						{Key: "audioCodecs", Value: float64(4071)},
						{Key: "videoCodecs", Value: float64(252)},
						{Key: "videoFunction", Value: float64(1)},
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
			}, msg)

			err = mrw.Write(&message.SetChunkSize{
				Value: 65536,
			})
			require.NoError(t, err)

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

			var streamKey string

			switch ca {
			case "stream key":
				streamKey = "key"

			case "stream key and query":
				streamKey = "key?key=val"
			}

			err = mrw.Write(&message.CommandAMF0{
				ChunkStreamID:   4,
				MessageStreamID: 0x1000000,
				Name:            "play",
				CommandID:       0,
				Arguments: []interface{}{
					nil,
					streamKey,
				},
			})
			require.NoError(t, err)

			<-done
		})
	}
}
