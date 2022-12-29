package core

import (
	"crypto/tls"
	"net"
	"os"
	"testing"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

func TestRTMPSource(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"tls",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := func() (net.Listener, error) {
				if ca == "plain" {
					return net.Listen("tcp", "127.0.0.1:1937")
				}

				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				var cert tls.Certificate
				cert, err = tls.LoadX509KeyPair(serverCertFpath, serverKeyFpath)
				require.NoError(t, err)

				return tls.Listen("tcp", "127.0.0.1:1937", &tls.Config{Certificates: []tls.Certificate{cert}})
			}()
			require.NoError(t, err)
			defer ln.Close()

			connected := make(chan struct{})
			received := make(chan struct{})
			done := make(chan struct{})

			go func() {
				nconn, err := ln.Accept()
				require.NoError(t, err)
				defer nconn.Close()
				conn := rtmp.NewConn(nconn)

				_, _, err = conn.InitializeServer()
				require.NoError(t, err)

				videoTrack := &format.H264{
					PayloadTyp: 96,
					SPS: []byte{ // 1920x1080 baseline
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
					},
					PPS:               []byte{0x08, 0x06, 0x07, 0x08},
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

				err = conn.WriteTracks(videoTrack, audioTrack)
				require.NoError(t, err)

				<-connected

				err = conn.WriteMessage(&message.MsgVideo{
					ChunkStreamID:   message.MsgVideoChunkStreamID,
					MessageStreamID: 0x1000000,
					IsKeyFrame:      true,
					H264Type:        flvio.AVC_NALU,
					Payload:         []byte{0x00, 0x00, 0x00, 0x04, 0x05, 0x02, 0x03, 0x04},
				})
				require.NoError(t, err)

				<-done
			}()

			if ca == "plain" {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtmp://localhost:1937/teststream\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.Close()
			} else {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtmps://localhost:1937/teststream\n" +
					"    sourceFingerprint: 33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.Close()
			}

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(medias, baseURL)
			require.NoError(t, err)

			c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
				require.Equal(t, []byte{
					0x18, 0x0, 0x19, 0x67, 0x42, 0xc0, 0x28, 0xd9,
					0x0, 0x78, 0x2, 0x27, 0xe5, 0x84, 0x0, 0x0,
					0x3, 0x0, 0x4, 0x0, 0x0, 0x3, 0x0, 0xf0,
					0x3c, 0x60, 0xc9, 0x20, 0x0, 0x4, 0x8, 0x6,
					0x7, 0x8, 0x0, 0x4, 0x5, 0x2, 0x3, 0x4,
				}, pkt.Payload)
				close(received)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			close(connected)
			<-received
			close(done)
		})
	}
}
