package core

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/webrtcpc"
	"github.com/bluenviron/mediamtx/internal/whip"
)

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...interface{}) {
}

type webRTCTestClient struct {
	pc             *webrtcpc.PeerConnection
	outgoingTrack1 *webrtc.TrackLocalStaticRTP
	outgoingTrack2 *webrtc.TrackLocalStaticRTP
	incomingTrack  chan *webrtc.TrackRemote
}

func newWebRTCTestClient(
	t *testing.T,
	hc *http.Client,
	ur string,
	publish bool,
) *webRTCTestClient {
	iceServers, err := whip.GetICEServers(context.Background(), hc, ur)
	require.NoError(t, err)

	c := &webRTCTestClient{}

	api, err := webrtcNewAPI(nil, nil, nil)
	require.NoError(t, err)

	pc, err := webrtcpc.New(iceServers, api, nilLogger{})
	require.NoError(t, err)

	var outgoingTrack1 *webrtc.TrackLocalStaticRTP
	var outgoingTrack2 *webrtc.TrackLocalStaticRTP
	var incomingTrack chan *webrtc.TrackRemote

	if publish {
		var err error
		outgoingTrack1, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			"vp8",
			webrtcStreamID,
		)
		require.NoError(t, err)

		_, err = pc.AddTrack(outgoingTrack1)
		require.NoError(t, err)

		outgoingTrack2, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
			},
			"opus",
			webrtcStreamID,
		)
		require.NoError(t, err)

		_, err = pc.AddTrack(outgoingTrack2)
		require.NoError(t, err)
	} else {
		incomingTrack = make(chan *webrtc.TrackRemote, 1)
		pc.OnTrack(func(trak *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
			incomingTrack <- trak
		})

		_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
		require.NoError(t, err)
	}

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	res, err := whip.PostOffer(context.Background(), hc, ur, &offer)
	require.NoError(t, err)

	err = pc.SetLocalDescription(offer)
	require.NoError(t, err)

	// test adding additional candidates, even if it is not mandatory here
outer:
	for {
		select {
		case c := <-pc.NewLocalCandidate():
			err := whip.PostCandidate(context.Background(), hc, ur, &offer, res.ETag, c)
			require.NoError(t, err)
		case <-pc.GatheringDone():
			break outer
		}
	}

	err = pc.SetRemoteDescription(*res.Answer)
	require.NoError(t, err)

	<-pc.Connected()

	if publish {
		err := outgoingTrack1.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: []byte{1},
		})
		require.NoError(t, err)

		err = outgoingTrack2.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 1123,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: []byte{2},
		})
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)
	}

	c.pc = pc
	c.outgoingTrack1 = outgoingTrack1
	c.outgoingTrack2 = outgoingTrack2
	c.incomingTrack = incomingTrack
	return c
}

func (c *webRTCTestClient) close() {
	c.pc.Close()
}

func TestWebRTCRead(t *testing.T) {
	for _, auth := range []string{
		"none",
		"internal",
		"external",
	} {
		t.Run("auth_"+auth, func(t *testing.T) {
			var conf string

			switch auth {
			case "none":
				conf = "paths:\n" +
					"  all:\n"

			case "internal":
				conf = "paths:\n" +
					"  all:\n" +
					"    readUser: myuser\n" +
					"    readPass: mypass\n"

			case "external":
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all:\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			var a *testHTTPAuthenticator
			if auth == "external" {
				a = newTestHTTPAuthenticator(t, "rtsp", "publish")
			}

			medi := &media.Media{
				Type: media.TypeVideo,
				Formats: []formats.Format{&formats.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			v := gortsplib.TransportTCP
			source := gortsplib.Client{
				Transport: &v,
			}
			err := source.StartRecording(
				"rtsp://testpublisher:testpass@localhost:8554/teststream?param=value", media.Medias{medi})
			require.NoError(t, err)
			defer source.Close()

			if auth == "external" {
				a.close()
				a = newTestHTTPAuthenticator(t, "webrtc", "read")
				defer a.close()
			}

			hc := &http.Client{Transport: &http.Transport{}}

			user := ""
			pass := ""

			switch auth {
			case "internal":
				user = "myuser"
				pass = "mypass"

			case "external":
				user = "testreader"
				pass = "testpass"
			}

			ur := "http://"
			if user != "" {
				ur += user + ":" + pass + "@"
			}
			ur += "localhost:8889/teststream/whep?param=value"

			c := newWebRTCTestClient(t, hc, ur, false)
			defer c.close()

			time.Sleep(500 * time.Millisecond)

			err = source.WritePacketRTP(medi, &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    96,
					SequenceNumber: 123,
					Timestamp:      45343,
					SSRC:           563423,
				},
				Payload: []byte{3},
			})
			require.NoError(t, err)

			trak := <-c.incomingTrack

			pkt, _, err := trak.ReadRTP()
			require.NoError(t, err)
			require.Equal(t, &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    100,
					SequenceNumber: pkt.SequenceNumber,
					Timestamp:      pkt.Timestamp,
					SSRC:           pkt.SSRC,
					CSRC:           []uint32{},
				},
				Payload: []byte{3},
			}, pkt)
		})
	}
}

func TestWebRTCReadNotFound(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	iceServers, err := whip.GetICEServers(context.Background(), hc, "http://localhost:8889/stream/whep")
	require.NoError(t, err)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "http://localhost:8889/stream/whep", bytes.NewReader([]byte(offer.SDP)))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/sdp")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestWebRTCPublish(t *testing.T) {
	for _, auth := range []string{
		"none",
		"internal",
		"external",
	} {
		t.Run("auth_"+auth, func(t *testing.T) {
			var conf string

			switch auth {
			case "none":
				conf = "paths:\n" +
					"  all:\n"

			case "internal":
				conf = "paths:\n" +
					"  all:\n" +
					"    publishUser: myuser\n" +
					"    publishPass: mypass\n"

			case "external":
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all:\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			var a *testHTTPAuthenticator
			if auth == "external" {
				a = newTestHTTPAuthenticator(t, "webrtc", "publish")
			}

			hc := &http.Client{Transport: &http.Transport{}}

			// preflight requests must always work, without authentication
			func() {
				req, err := http.NewRequest("OPTIONS", "http://localhost:8889/teststream/whip", nil)
				require.NoError(t, err)

				req.Header.Set("Access-Control-Request-Method", "OPTIONS")

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNoContent, res.StatusCode)

				if auth != "none" {
					_, ok := res.Header["Link"]
					require.Equal(t, false, ok)
				}
			}()

			user := ""
			pass := ""

			switch auth {
			case "internal":
				user = "myuser"
				pass = "mypass"

			case "external":
				user = "testpublisher"
				pass = "testpass"
			}

			ur := "http://"
			if user != "" {
				ur += user + ":" + pass + "@"
			}
			ur += "localhost:8889/teststream/whip?param=value"

			s := newWebRTCTestClient(t, hc, ur, true)
			defer s.close()

			if auth == "external" {
				a.close()
				a = newTestHTTPAuthenticator(t, "rtsp", "read")
				defer a.close()
			}

			c := gortsplib.Client{
				OnDecodeError: func(err error) {
					panic(err)
				},
			}

			u, err := url.Parse("rtsp://testreader:testpass@127.0.0.1:8554/teststream?param=value")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			var forma *formats.VP8
			medi := medias.FindFormat(&forma)

			_, err = c.Setup(medi, baseURL, 0, 0)
			require.NoError(t, err)

			received := make(chan struct{})

			c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
				require.Equal(t, []byte{3}, pkt.Payload)
				close(received)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			err = s.outgoingTrack1.WriteRTP(&rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    96,
					SequenceNumber: 124,
					Timestamp:      45343,
					SSRC:           563423,
				},
				Payload: []byte{3},
			})
			require.NoError(t, err)

			<-received
		})
	}
}
