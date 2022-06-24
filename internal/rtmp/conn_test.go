package rtmp

import (
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	nh264 "github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/handshake"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

func splitPath(u *url.URL) (app, stream string) {
	nu := *u
	nu.ForceQuery = false

	pathsegs := strings.Split(nu.RequestURI(), "/")
	if len(pathsegs) == 2 {
		app = pathsegs[1]
	}
	if len(pathsegs) == 3 {
		app = pathsegs[1]
		stream = pathsegs[2]
	}
	if len(pathsegs) > 3 {
		app = strings.Join(pathsegs[1:3], "/")
		stream = strings.Join(pathsegs[3:], "/")
	}
	return
}

func getTcURL(u string) string {
	ur, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	app, _ := splitPath(ur)
	nu := *ur
	nu.RawQuery = ""
	nu.Path = "/"
	return nu.String() + app
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

	for _, ca := range []string{
		"standard",
		"metadata without codec id",
		"missing metadata",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				conn, err := ln.Accept()
				require.NoError(t, err)
				defer conn.Close()

				rconn := NewServerConn(conn)
				err = rconn.ServerHandshake()
				require.NoError(t, err)

				videoTrack, audioTrack, err := rconn.ReadTracks()
				require.NoError(t, err)

				switch ca {
				case "standard":
					require.Equal(t, &gortsplib.TrackH264{
						PayloadType: 96,
						SPS:         sps,
						PPS:         pps,
					}, videoTrack)

					require.Equal(t, &gortsplib.TrackAAC{
						PayloadType: 97,
						Config: &aac.MPEG4AudioConfig{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						},
						SizeLength:       13,
						IndexLength:      3,
						IndexDeltaLength: 3,
					}, audioTrack)

				case "metadata without codec id":
					require.Equal(t, &gortsplib.TrackH264{
						PayloadType: 96,
						SPS:         sps,
						PPS:         pps,
					}, videoTrack)

					require.Equal(t, &gortsplib.TrackAAC{
						PayloadType: 97,
						Config: &aac.MPEG4AudioConfig{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						},
						SizeLength:       13,
						IndexLength:      3,
						IndexDeltaLength: 3,
					}, audioTrack)

				case "missing metadata":
					require.Equal(t, &gortsplib.TrackH264{
						PayloadType: 96,
						SPS:         sps,
						PPS:         pps,
					}, videoTrack)

					require.Equal(t, &gortsplib.TrackAAC{
						PayloadType: 97,
						Config: &aac.MPEG4AudioConfig{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						},
						SizeLength:       13,
						IndexLength:      3,
						IndexDeltaLength: 3,
					}, audioTrack)
				}

				close(done)
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()
			bc := bytecounter.NewReadWriter(conn)

			// C->S handshake C0
			err = handshake.C0S0{}.Write(conn)
			require.NoError(t, err)

			// C->S handshake C1
			c1 := handshake.C1S1{}
			err = c1.Write(conn, true)
			require.NoError(t, err)

			// S->C handshake S0
			err = handshake.C0S0{}.Read(bc)
			require.NoError(t, err)

			// S->C handshake S1
			s1 := handshake.C1S1{}
			err = s1.Read(bc, false)
			require.NoError(t, err)

			// S->C handshake S2
			err = (&handshake.C2S2{Digest: c1.Digest}).Read(bc)
			require.NoError(t, err)

			// C->S handshake C2
			err = handshake.C2S2{Digest: s1.Digest}.Write(conn)
			require.NoError(t, err)

			mrw := message.NewReadWriter(bc)

			// C->S connect
			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Payload: []interface{}{
					"connect",
					1,
					flvio.AMFMap{
						{K: "app", V: "/stream"},
						{K: "flashVer", V: "LNX 9,0,124,2"},
						{K: "tcUrl", V: getTcURL("rtmp://127.0.0.1:9121/stream")},
						{K: "fpad", V: false},
						{K: "capabilities", V: 15},
						{K: "audioCodecs", V: 4071},
						{K: "videoCodecs", V: 252},
						{K: "videoFunction", V: 1},
					},
				},
			})
			require.NoError(t, err)

			// S->C window acknowledgement size
			msg, err := mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetWindowAckSize{
				Value: 2500000,
			}, msg)

			// S->C set peer bandwidth
			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetPeerBandwidth{
				Value: 2500000,
				Type:  2,
			}, msg)

			// S->C set chunk size
			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgSetChunkSize{
				Value: 65536,
			}, msg)

			// S->C result
			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Payload: []interface{}{
					"_result",
					float64(1),
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

			// C->S set chunk size
			err = mrw.Write(&message.MsgSetChunkSize{
				Value: 65536,
			})
			require.NoError(t, err)

			// C->S releaseStream
			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Payload: []interface{}{
					"releaseStream",
					float64(2),
					nil,
					"",
				},
			})
			require.NoError(t, err)

			// C->S FCPublish
			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Payload: []interface{}{
					"FCPublish",
					float64(3),
					nil,
					"",
				},
			})
			require.NoError(t, err)

			// C->S createStream
			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Payload: []interface{}{
					"createStream",
					float64(4),
					nil,
				},
			})
			require.NoError(t, err)

			// S->C result
			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgCommandAMF0{
				ChunkStreamID: 3,
				Payload: []interface{}{
					"_result",
					float64(4),
					nil,
					float64(1),
				},
			}, msg)

			// C->S publish
			err = mrw.Write(&message.MsgCommandAMF0{
				ChunkStreamID:   8,
				MessageStreamID: 1,
				Payload: []interface{}{
					"publish",
					float64(5),
					nil,
					"",
					"live",
				},
			})
			require.NoError(t, err)

			// S->C onStatus
			msg, err = mrw.Read()
			require.NoError(t, err)
			require.Equal(t, &message.MsgCommandAMF0{
				ChunkStreamID:   5,
				MessageStreamID: 16777216,
				Payload: []interface{}{
					"onStatus",
					float64(5),
					nil,
					flvio.AMFMap{
						{K: "level", V: "status"},
						{K: "code", V: "NetStream.Publish.Start"},
						{K: "description", V: "publish start"},
					},
				},
			}, msg)

			switch ca {
			case "standard":
				// C->S metadata
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

				// C->S H264 decoder config
				codec := nh264.Codec{
					SPS: map[int][]byte{
						0: sps,
					},
					PPS: map[int][]byte{
						0: pps,
					},
				}
				b := make([]byte, 128)
				var n int
				codec.ToConfig(b, &n)
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   6,
					MessageStreamID: 1,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         b[:n],
				})
				require.NoError(t, err)

				// C->S AAC decoder config
				enc, err := aac.MPEG4AudioConfig{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_SEQHDR,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "metadata without codec id":
				// C->S metadata
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

				// C->S H264 decoder config
				codec := nh264.Codec{
					SPS: map[int][]byte{
						0: sps,
					},
					PPS: map[int][]byte{
						0: pps,
					},
				}
				b := make([]byte, 128)
				var n int
				codec.ToConfig(b, &n)
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   6,
					MessageStreamID: 1,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         b[:n],
				})
				require.NoError(t, err)

				// C->S AAC decoder config
				enc, err := aac.MPEG4AudioConfig{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_SEQHDR,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "missing metadata":
				// C->S H264 decoder config
				codec := nh264.Codec{
					SPS: map[int][]byte{
						0: sps,
					},
					PPS: map[int][]byte{
						0: pps,
					},
				}
				b := make([]byte, 128)
				var n int
				codec.ToConfig(b, &n)
				err = mrw.Write(&message.MsgVideo{
					ChunkStreamID:   6,
					MessageStreamID: 1,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_SEQHDR,
					Payload:         b[:n],
				})
				require.NoError(t, err)

				// C->S AAC decoder config
				enc, err := aac.MPEG4AudioConfig{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)
				err = mrw.Write(&message.MsgAudio{
					ChunkStreamID:   4,
					MessageStreamID: 1,
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

		rconn := NewServerConn(conn)
		err = rconn.ServerHandshake()
		require.NoError(t, err)

		videoTrack := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
		}

		audioTrack := &gortsplib.TrackAAC{
			PayloadType: 97,
			Config: &aac.MPEG4AudioConfig{
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

	// C->S handshake C0
	err = handshake.C0S0{}.Write(conn)
	require.NoError(t, err)

	// C-> handshake C1
	c1 := handshake.C1S1{}
	err = c1.Write(conn, true)
	require.NoError(t, err)

	// S->C handshake S0
	err = handshake.C0S0{}.Read(bc)
	require.NoError(t, err)

	// S->C handshake S1
	s1 := handshake.C1S1{}
	err = s1.Read(bc, false)
	require.NoError(t, err)

	// S->C handshake S2
	err = (&handshake.C2S2{Digest: c1.Digest}).Read(bc)
	require.NoError(t, err)

	// C->S handshake C2
	err = handshake.C2S2{Digest: s1.Digest}.Write(conn)
	require.NoError(t, err)

	mrw := message.NewReadWriter(bc)

	// C->S connect
	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID: 3,
		Payload: []interface{}{
			"connect",
			1,
			flvio.AMFMap{
				{K: "app", V: "/stream"},
				{K: "flashVer", V: "LNX 9,0,124,2"},
				{K: "tcUrl", V: getTcURL("rtmp://127.0.0.1:9121/stream")},
				{K: "fpad", V: false},
				{K: "capabilities", V: 15},
				{K: "audioCodecs", V: 4071},
				{K: "videoCodecs", V: 252},
				{K: "videoFunction", V: 1},
			},
		},
	})
	require.NoError(t, err)

	// S->C window acknowledgement size
	msg, err := mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgSetWindowAckSize{
		Value: 2500000,
	}, msg)

	// S->C set peer bandwidth
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgSetPeerBandwidth{
		Value: 2500000,
		Type:  2,
	}, msg)

	// S->C set chunk size
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgSetChunkSize{
		Value: 65536,
	}, msg)

	// S->C result
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID: 3,
		Payload: []interface{}{
			"_result",
			float64(1),
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

	// C->S window acknowledgement size
	err = mrw.Write(&message.MsgSetWindowAckSize{
		Value: 2500000,
	})
	require.NoError(t, err)

	// C->S set chunk size
	err = mrw.Write(&message.MsgSetChunkSize{
		Value: 65536,
	})
	require.NoError(t, err)

	// C->S createStream
	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID: 3,
		Payload: []interface{}{
			"createStream",
			float64(2),
			nil,
		},
	})
	require.NoError(t, err)

	// S->C result
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID: 3,
		Payload: []interface{}{
			"_result",
			float64(2),
			nil,
			float64(1),
		},
	}, msg)

	// C->S getStreamLength
	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID: 8,
		Payload: []interface{}{
			"getStreamLength",
			float64(3),
			nil,
			"",
		},
	})
	require.NoError(t, err)

	// C->S play
	err = mrw.Write(&message.MsgCommandAMF0{
		ChunkStreamID: 8,
		Payload: []interface{}{
			"play",
			float64(4),
			nil,
			"",
			float64(-2000),
		},
	})
	require.NoError(t, err)

	// S->C event "stream is recorded"
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgUserControlStreamIsRecorded{
		StreamID: 1,
	}, msg)

	// S->C event "stream begin 1"
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgUserControlStreamBegin{
		StreamID: 1,
	}, msg)

	// S->C onStatus
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 16777216,
		Payload: []interface{}{
			"onStatus",
			float64(4),
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Play.Reset"},
				{K: "description", V: "play reset"},
			},
		},
	}, msg)

	// S->C onStatus
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 16777216,
		Payload: []interface{}{
			"onStatus",
			float64(4),
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Play.Start"},
				{K: "description", V: "play start"},
			},
		},
	}, msg)

	// S->C onStatus
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 16777216,
		Payload: []interface{}{
			"onStatus",
			float64(4),
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Data.Start"},
				{K: "description", V: "data start"},
			},
		},
	}, msg)

	// S->C onStatus
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgCommandAMF0{
		ChunkStreamID:   5,
		MessageStreamID: 16777216,
		Payload: []interface{}{
			"onStatus",
			float64(4),
			nil,
			flvio.AMFMap{
				{K: "level", V: "status"},
				{K: "code", V: "NetStream.Play.PublishNotify"},
				{K: "description", V: "publish notify"},
			},
		},
	}, msg)

	// S->C onMetadata
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgDataAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 16777216,
		Payload: []interface{}{
			"onMetaData",
			flvio.AMFMap{
				{K: "videodatarate", V: float64(0)},
				{K: "videocodecid", V: float64(7)},
				{K: "audiodatarate", V: float64(0)},
				{K: "audiocodecid", V: float64(10)},
			},
		},
	}, msg)

	// S->C H264 decoder config
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgVideo{
		ChunkStreamID:   6,
		MessageStreamID: 16777216,
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

	// S->C AAC decoder config
	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.MsgAudio{
		ChunkStreamID:   4,
		MessageStreamID: 16777216,
		Rate:            flvio.SOUND_44Khz,
		Depth:           flvio.SOUND_16BIT,
		Channels:        flvio.SOUND_STEREO,
		AACType:         flvio.AAC_SEQHDR,
		Payload:         []byte{0x12, 0x10},
	}, msg)
}
