package rtmp

import (
	"bytes"
	"net"
	"net/url"
	"testing"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/h264"
	"github.com/aler9/gortsplib/v2/pkg/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/h264conf"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/handshake"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

func TestInitializeClient(t *testing.T) {
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

				err = handshake.DoServer(bc, true)
				require.NoError(t, err)

				mrw := message.NewReadWriter(bc, true)

				msg, err := mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.MsgSetWindowAckSize{
					Value: 2500000,
				}, msg)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.MsgSetPeerBandwidth{
					Value: 2500000,
					Type:  2,
				}, msg)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.MsgSetChunkSize{
					Value: 65536,
				}, msg)

				msg, err = mrw.Read()
				require.NoError(t, err)
				require.Equal(t, &message.MsgCommandAMF0{
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

				err = mrw.Write(&message.MsgCommandAMF0{
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
					require.Equal(t, &message.MsgCommandAMF0{
						ChunkStreamID: 3,
						Name:          "createStream",
						CommandID:     2,
						Arguments: []interface{}{
							nil,
						},
					}, msg)

					err = mrw.Write(&message.MsgCommandAMF0{
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
					require.Equal(t, &message.MsgUserControlSetBufferLength{
						BufferLength: 0x64,
					}, msg)

					msg, err = mrw.Read()
					require.NoError(t, err)
					require.Equal(t, &message.MsgCommandAMF0{
						ChunkStreamID:   4,
						MessageStreamID: 0x1000000,
						Name:            "play",
						CommandID:       3,
						Arguments: []interface{}{
							nil,
							"",
						},
					}, msg)

					err = mrw.Write(&message.MsgCommandAMF0{
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
					require.Equal(t, &message.MsgCommandAMF0{
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
					require.Equal(t, &message.MsgCommandAMF0{
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
					require.Equal(t, &message.MsgCommandAMF0{
						ChunkStreamID: 3,
						Name:          "createStream",
						CommandID:     4,
						Arguments: []interface{}{
							nil,
						},
					}, msg)

					err = mrw.Write(&message.MsgCommandAMF0{
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
					require.Equal(t, &message.MsgCommandAMF0{
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

					err = mrw.Write(&message.MsgCommandAMF0{
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
			conn := NewConn(nconn)

			err = conn.InitializeClient(u, ca == "publish")
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

func TestInitializeServer(t *testing.T) {
	for _, ca := range []string{"read", "publish"} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				nconn, err := ln.Accept()
				require.NoError(t, err)
				defer nconn.Close()

				conn := NewConn(nconn)
				u, isPublishing, err := conn.InitializeServer()
				require.NoError(t, err)
				require.Equal(t, &url.URL{
					Scheme: "rtmp",
					Host:   "127.0.0.1:9121",
					Path:   "//stream/",
				}, u)
				require.Equal(t, ca == "publish", isPublishing)

				close(done)
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()
			bc := bytecounter.NewReadWriter(conn)

			err = handshake.DoClient(bc, true)
			require.NoError(t, err)

			mrw := message.NewReadWriter(bc, true)

			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Name:          "connect",
				CommandID:     1,
				Arguments: []interface{}{
					flvio.AMFMap{
						{K: "app", V: "/stream"},
						{K: "flashVer", V: "LNX 9,0,124,2"},
						{K: "tcUrl", V: "rtmp://127.0.0.1:9121/stream"},
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
			require.Equal(t, &message.MsgSetWindowAckSize{
				Value: 2500000,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetPeerBandwidth{
				Value: 2500000,
				Type:  2,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetChunkSize{
				Value: 65536,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgCommandAMF0{
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

			err = mrw.Write(&message.MsgSetChunkSize{
				Value: 65536,
			})
			require.NoError(t, err)

			if ca == "read" {
				err = mrw.Write(&message.MsgCommandAMF0{
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
				require.Equal(t, &message.MsgCommandAMF0{
					ChunkStreamID: 3,
					Name:          "_result",
					CommandID:     2,
					Arguments: []interface{}{
						nil,
						float64(1),
					},
				}, msg)

				err = mrw.Write(&message.MsgUserControlSetBufferLength{
					BufferLength: 0x64,
				})
				require.NoError(t, err)

				err = mrw.Write(&message.MsgCommandAMF0{
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
				err = mrw.Write(&message.MsgCommandAMF0{
					ChunkStreamID: 3,
					Name:          "releaseStream",
					CommandID:     2,
					Arguments: []interface{}{
						nil,
						"",
					},
				})
				require.NoError(t, err)

				err = mrw.Write(&message.MsgCommandAMF0{
					ChunkStreamID: 3,
					Name:          "FCPublish",
					CommandID:     3,
					Arguments: []interface{}{
						nil,
						"",
					},
				})
				require.NoError(t, err)

				err = mrw.Write(&message.MsgCommandAMF0{
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
				require.Equal(t, &message.MsgCommandAMF0{
					ChunkStreamID: 3,
					Name:          "_result",
					CommandID:     4,
					Arguments: []interface{}{
						nil,
						float64(1),
					},
				}, msg)

				err = mrw.Write(&message.MsgCommandAMF0{
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

func TestReadTracks(t *testing.T) {
	sps := []byte{
		0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
		0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
		0x00, 0x03, 0x00, 0x3d, 0x08,
	}

	pps := []byte{
		0x68, 0xee, 0x3c, 0x80,
	}

	for _, ca := range []struct {
		name       string
		videoTrack format.Format
		audioTrack format.Format
	}{
		{
			"video+audio",
			&format.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"video",
			&format.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			(*format.MPEG4Audio)(nil),
		},
		{
			"metadata without codec id",
			&format.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"missing metadata",
			&format.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"obs studio h265",
			&format.H265{
				PayloadTyp: 96,
				VPS: []byte{
					0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40,
					0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
					0x03, 0x00, 0x00, 0x03, 0x00, 0x7b, 0xac, 0x09,
				},
				SPS: []byte{
					0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03,
					0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
					0x03, 0x00, 0x7b, 0xa0, 0x03, 0xc0, 0x80, 0x11,
					0x07, 0xcb, 0x96, 0xb4, 0xa4, 0x25, 0x92, 0xe3,
					0x01, 0x6a, 0x02, 0x02, 0x02, 0x08, 0x00, 0x00,
					0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x01, 0xe3,
					0x00, 0x2e, 0xf2, 0x88, 0x00, 0x09, 0x89, 0x60,
					0x00, 0x04, 0xc4, 0xb4, 0x20,
				},
				PPS: []byte{
					0x44, 0x01, 0xc0, 0xf7, 0xc0, 0xcc, 0x90,
				},
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				conn, err := ln.Accept()
				require.NoError(t, err)
				defer conn.Close()

				rconn := NewConn(conn)
				_, _, err = rconn.InitializeServer()
				require.NoError(t, err)

				videoTrack, audioTrack, err := rconn.ReadTracks()
				require.NoError(t, err)
				require.Equal(t, ca.videoTrack, videoTrack)
				require.Equal(t, ca.audioTrack, audioTrack)
				close(done)
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()
			bc := bytecounter.NewReadWriter(conn)

			err = handshake.DoClient(bc, true)
			require.NoError(t, err)

			mrw := message.NewReadWriter(bc, true)

			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Name:          "connect",
				CommandID:     1,
				Arguments: []interface{}{
					flvio.AMFMap{
						{K: "app", V: "/stream"},
						{K: "flashVer", V: "LNX 9,0,124,2"},
						{K: "tcUrl", V: "rtmp://127.0.0.1:9121/stream"},
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
			require.Equal(t, &message.MsgSetWindowAckSize{
				Value: 2500000,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetPeerBandwidth{
				Value: 2500000,
				Type:  2,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetChunkSize{
				Value: 65536,
			}, msg)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgCommandAMF0{
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

			err = mrw.Write(&message.MsgSetChunkSize{
				Value: 65536,
			})
			require.NoError(t, err)

			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Name:          "releaseStream",
				CommandID:     2,
				Arguments: []interface{}{
					nil,
					"",
				},
			})
			require.NoError(t, err)

			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Name:          "FCPublish",
				CommandID:     3,
				Arguments: []interface{}{
					nil,
					"",
				},
			})
			require.NoError(t, err)

			err = mrw.Write(&message.MsgCommandAMF0{
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
			require.Equal(t, &message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Name:          "_result",
				CommandID:     4,
				Arguments: []interface{}{
					nil,
					float64(1),
				},
			}, msg)

			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID:   8,
				MessageStreamID: 1,
				Name:            "publish",
				CommandID:       5,
				Arguments: []interface{}{
					nil,
					"",
					"live",
				},
			})
			require.NoError(t, err)

			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgCommandAMF0{
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
			}, msg)

			switch ca.name {
			case "video+audio":
				err = mrw.Write(&message.MsgDataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(codecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(codecAAC),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   message.MsgVideoChunkStreamID,
					MessageStreamID: 0x1000000,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   message.MsgAudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_SEQHDR,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "video":
				err = mrw.Write(&message.MsgDataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(codecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(0),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   message.MsgVideoChunkStreamID,
					MessageStreamID: 0x1000000,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         buf,
				})
				require.NoError(t, err)

			case "metadata without codec id":
				err = mrw.Write(&message.MsgDataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "width",
								V: float64(2688),
							},
							{
								K: "height",
								V: float64(1520),
							},
							{
								K: "framerate",
								V: float64(0o25),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   message.MsgVideoChunkStreamID,
					MessageStreamID: 0x1000000,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   message.MsgAudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_SEQHDR,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "missing metadata":
				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   message.MsgVideoChunkStreamID,
					MessageStreamID: 0x1000000,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   message.MsgAudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_SEQHDR,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "obs studio h265":
				err = mrw.Write(&message.MsgDataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(codecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(codecAAC),
							},
						},
					},
				})
				require.NoError(t, err)

				avcc, err := h264.AVCCMarshal([][]byte{
					{ // VPS
						0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40,
						0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
						0x03, 0x00, 0x00, 0x03, 0x00, 0x7b, 0xac, 0x09,
					},
					{ // SPS
						0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03,
						0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
						0x03, 0x00, 0x7b, 0xa0, 0x03, 0xc0, 0x80, 0x11,
						0x07, 0xcb, 0x96, 0xb4, 0xa4, 0x25, 0x92, 0xe3,
						0x01, 0x6a, 0x02, 0x02, 0x02, 0x08, 0x00, 0x00,
						0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x01, 0xe3,
						0x00, 0x2e, 0xf2, 0x88, 0x00, 0x09, 0x89, 0x60,
						0x00, 0x04, 0xc4, 0xb4, 0x20,
					},
					{
						// PPS
						0x44, 0x01, 0xc0, 0xf7, 0xc0, 0xcc, 0x90,
					},
				})
				require.NoError(t, err)

				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   message.MsgVideoChunkStreamID,
					MessageStreamID: 0x1000000,
					IsKeyFrame:      true,
					H264Type:        1,
					Payload:         avcc,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   message.MsgAudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_SEQHDR,
					Payload:         enc,
				})
				require.NoError(t, err)
			}

			<-done
		})
	}
}

func TestWriteTracks(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9121")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		require.NoError(t, err)
		defer conn.Close()

		rconn := NewConn(conn)
		_, _, err = rconn.InitializeServer()
		require.NoError(t, err)

		videoTrack := &format.H264{
			PayloadTyp: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
			PacketizationMode: 1,
		}

		audioTrack := &format.MPEG4Audio{
			PayloadTyp: 96,
			Config: &mpeg4audio.Config{
				Type:         2,
				SampleRate:   44100,
				ChannelCount: 2,
			},
			SizeLength:       13,
			IndexLength:      3,
			IndexDeltaLength: 3,
		}

		err = rconn.WriteTracks(videoTrack, audioTrack)
		require.NoError(t, err)
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:9121")
	require.NoError(t, err)
	defer conn.Close()
	bc := bytecounter.NewReadWriter(conn)

	err = handshake.DoClient(bc, true)
	require.NoError(t, err)

	mrw := message.NewReadWriter(bc, true)

	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID: 3,
		Name:          "connect",
		CommandID:     1,
		Arguments: []interface{}{
			flvio.AMFMap{
				{K: "app", V: "/stream"},
				{K: "flashVer", V: "LNX 9,0,124,2"},
				{K: "tcUrl", V: "rtmp://127.0.0.1:9121/stream"},
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
	require.Equal(t, &message.MsgSetWindowAckSize{
		Value: 2500000,
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgSetPeerBandwidth{
		Value: 2500000,
		Type:  2,
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgSetChunkSize{
		Value: 65536,
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
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

	err = mrw.Write(&message.MsgSetWindowAckSize{
		Value: 2500000,
	})
	require.NoError(t, err)

	err = mrw.Write(&message.MsgSetChunkSize{
		Value: 65536,
	})
	require.NoError(t, err)

	err = mrw.Write(&message.MsgCommandAMF0{
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
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID: 3,
		Name:          "_result",
		CommandID:     2,
		Arguments: []interface{}{
			nil,
			float64(1),
		},
	}, msg)

	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID: 8,
		Name:          "getStreamLength",
		CommandID:     3,
		Arguments: []interface{}{
			nil,
			"",
		},
	})
	require.NoError(t, err)

	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID:   8,
		MessageStreamID: 0x1000000,
		Name:            "play",
		CommandID:       4,
		Arguments: []interface{}{
			nil,
			"",
			float64(-2000),
		},
	})
	require.NoError(t, err)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgUserControlStreamIsRecorded{
		StreamID: 1,
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgUserControlStreamBegin{
		StreamID: 1,
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 0x1000000,
		Name:            "onStatus",
		CommandID:       4,
		Arguments: []interface{}{
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Play.Reset"},
				{K: "description", V: "play reset"},
			},
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 0x1000000,
		Name:            "onStatus",
		CommandID:       4,
		Arguments: []interface{}{
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Play.Start"},
				{K: "description", V: "play start"},
			},
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 0x1000000,
		Name:            "onStatus",
		CommandID:       4,
		Arguments: []interface{}{
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Data.Start"},
				{K: "description", V: "data start"},
			},
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 0x1000000,
		Name:            "onStatus",
		CommandID:       4,
		Arguments: []interface{}{
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Play.PublishNotify"},
				{K: "description", V: "publish notify"},
			},
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgDataAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Payload: []interface{}{
			"@setDataFrame",
			"onMetaData",
			flvio.AMFMap{
				{K: "videodatarate", V: float64(0)},
				{K: "videocodecid", V: float64(7)},
				{K: "audiodatarate", V: float64(0)},
				{K: "audiocodecid", V: float64(10)},
			},
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgVideo{
		ChunkStreamID:   message.MsgVideoChunkStreamID,
		MessageStreamID: 0x1000000,
		IsKeyFrame:      true,
		H264Type:        flvio.AVC_SEQHDR,
		Payload: []byte{
			0x1, 0x64, 0x0,
			0xc, 0xff, 0xe1, 0x0, 0x15, 0x67, 0x64, 0x0,
			0xc, 0xac, 0x3b, 0x50, 0xb0, 0x4b, 0x42, 0x0,
			0x0, 0x3, 0x0, 0x2, 0x0, 0x0, 0x3, 0x0,
			0x3d, 0x8, 0x1, 0x0, 0x4, 0x68, 0xee, 0x3c,
			0x80,
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgAudio{
		ChunkStreamID:   message.MsgAudioChunkStreamID,
		MessageStreamID: 0x1000000,
		Rate:            flvio.SOUND_44Khz,
		Depth:           flvio.SOUND_16BIT,
		Channels:        flvio.SOUND_STEREO,
		AACType:         flvio.AAC_SEQHDR,
		Payload:         []byte{0x12, 0x10},
	}, msg)
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

	conn := NewConn(&buf)

	for n := 0; n < b.N; n++ {
		conn.ReadMessage()
	}
}
