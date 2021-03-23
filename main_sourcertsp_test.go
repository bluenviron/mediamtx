package main

import (
	"bufio"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestSourceRTSP(t *testing.T) {
	for _, source := range []string{
		"udp",
		"tcp",
		"tls",
	} {
		t.Run(source, func(t *testing.T) {
			switch source {
			case "udp", "tcp":
				p1, ok := testProgram("rtmpDisable: yes\n" +
					"rtspPort: 8555\n" +
					"rtpPort: 8100\n" +
					"rtcpPort: 8101\n" +
					"paths:\n" +
					"  all:\n" +
					"    readUser: testuser\n" +
					"    readPass: testpass\n")
				require.Equal(t, true, ok)
				defer p1.close()

				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.mkv",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport", "udp",
					"rtsp://" + ownDockerIP + ":8555/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				p2, ok := testProgram("rtmpDisable: yes\n" +
					"paths:\n" +
					"  proxied:\n" +
					"    source: rtsp://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceProtocol: " + source[len(""):] + "\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p2.close()

			case "tls":
				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := testProgram("rtmpDisable: yes\n" +
					"rtspPort: 8555\n" +
					"rtpPort: 8100\n" +
					"rtcpPort: 8101\n" +
					"readTimeout: 20s\n" +
					"protocols: [tcp]\n" +
					"encryption: yes\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n" +
					"paths:\n" +
					"  all:\n" +
					"    readUser: testuser\n" +
					"    readPass: testpass\n")
				require.Equal(t, true, ok)
				defer p.close()

				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.mkv",
					"-c", "copy",
					"-f", "rtsp",
					"rtsps://" + ownDockerIP + ":8555/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

				p2, ok := testProgram("rtmpDisable: yes\n" +
					"paths:\n" +
					"  proxied:\n" +
					"    source: rtsps://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p2.close()
			}

			time.Sleep(1 * time.Second)

			cnt3, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8554/proxied",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt3.close()
			require.Equal(t, 0, cnt3.wait())
		})
	}
}

func TestSourceRTSPRTPInfo(t *testing.T) {
	l, err := net.Listen("tcp", "localhost:8555")
	require.NoError(t, err)
	defer l.Close()

	serverDone := make(chan struct{})
	defer func() { <-serverDone }()
	go func() {
		defer close(serverDone)

		conn, err := l.Accept()
		require.NoError(t, err)
		bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

		var req base.Request
		err = req.Read(bconn.Reader)
		require.NoError(t, err)
		require.Equal(t, base.Options, req.Method)

		err = base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Public": base.HeaderValue{strings.Join([]string{
					string(base.Describe),
					string(base.Setup),
					string(base.Play),
				}, ", ")},
			},
		}.Write(bconn.Writer)
		require.NoError(t, err)

		err = req.Read(bconn.Reader)
		require.NoError(t, err)
		require.Equal(t, base.Describe, req.Method)

		track1, err := gortsplib.NewTrackH264(96, []byte("123456"), []byte("123456"))
		require.NoError(t, err)

		track2, err := gortsplib.NewTrackH264(96, []byte("123456"), []byte("123456"))
		require.NoError(t, err)

		err = base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Content-Type": base.HeaderValue{"application/sdp"},
			},
			Body: gortsplib.Tracks{track1, track2}.Write(),
		}.Write(bconn.Writer)
		require.NoError(t, err)

		err = req.Read(bconn.Reader)
		require.NoError(t, err)
		require.Equal(t, base.Setup, req.Method)

		var th headers.Transport
		err = th.Read(req.Header["Transport"])
		require.NoError(t, err)

		err = base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Transport": headers.Transport{
					Protocol: gortsplib.StreamProtocolTCP,
					Delivery: func() *base.StreamDelivery {
						v := base.StreamDeliveryUnicast
						return &v
					}(),
					ClientPorts:    th.ClientPorts,
					InterleavedIDs: &[2]int{0, 1},
				}.Write(),
			},
		}.Write(bconn.Writer)
		require.NoError(t, err)

		err = req.Read(bconn.Reader)
		require.NoError(t, err)
		require.Equal(t, base.Setup, req.Method)

		err = th.Read(req.Header["Transport"])
		require.NoError(t, err)

		err = base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Transport": headers.Transport{
					Protocol: gortsplib.StreamProtocolTCP,
					Delivery: func() *base.StreamDelivery {
						v := base.StreamDeliveryUnicast
						return &v
					}(),
					ClientPorts:    th.ClientPorts,
					InterleavedIDs: &[2]int{2, 3},
				}.Write(),
			},
		}.Write(bconn.Writer)
		require.NoError(t, err)

		err = req.Read(bconn.Reader)
		require.NoError(t, err)
		require.Equal(t, base.Play, req.Method)

		err = base.Response{
			StatusCode: base.StatusOK,
			Header:     base.Header{},
		}.Write(bconn.Writer)
		require.NoError(t, err)

		pkt := rtp.Packet{
			Header: rtp.Header{
				Version:        0x80,
				PayloadType:    96,
				SequenceNumber: 34254,
				Timestamp:      156457686,
				SSRC:           96342362,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}

		buf, err := pkt.Marshal()
		require.NoError(t, err)

		err = base.InterleavedFrame{
			TrackID:    1,
			StreamType: gortsplib.StreamTypeRTP,
			Payload:    buf,
		}.Write(bconn.Writer)
		require.NoError(t, err)

		pkt = rtp.Packet{
			Header: rtp.Header{
				Version:        0x80,
				PayloadType:    96,
				SequenceNumber: 87,
				Timestamp:      756436454,
				SSRC:           96342362,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}

		buf, err = pkt.Marshal()
		require.NoError(t, err)

		err = base.InterleavedFrame{
			TrackID:    0,
			StreamType: gortsplib.StreamTypeRTP,
			Payload:    buf,
		}.Write(bconn.Writer)
		require.NoError(t, err)

		err = req.Read(bconn.Reader)
		require.NoError(t, err)
		require.Equal(t, base.Teardown, req.Method)

		err = base.Response{
			StatusCode: base.StatusOK,
		}.Write(bconn.Writer)
		require.NoError(t, err)

		conn.Close()
	}()

	p1, ok := testProgram("rtmpDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://localhost:8555/stream\n" +
		"    sourceProtocol: tcp\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1000 * time.Millisecond)

	dest, err := gortsplib.DialRead("rtsp://127.0.1.2:8554/proxied")
	require.NoError(t, err)
	defer dest.Close()

	require.Equal(t, &headers.RTPInfo{
		&headers.RTPInfoEntry{
			URL: &base.URL{
				Scheme: "rtsp",
				Host:   "127.0.1.2:8554",
				Path:   "/proxied/trackID=0",
			},
			SequenceNumber: func() *uint16 {
				v := uint16(87)
				return &v
			}(),
			Timestamp: (*dest.RTPInfo())[0].Timestamp,
		},
		&headers.RTPInfoEntry{
			URL: &base.URL{
				Scheme: "rtsp",
				Host:   "127.0.1.2:8554",
				Path:   "/proxied/trackID=1",
			},
			SequenceNumber: func() *uint16 {
				v := uint16(34254)
				return &v
			}(),
			Timestamp: (*dest.RTPInfo())[1].Timestamp,
		},
	}, dest.RTPInfo())
}
