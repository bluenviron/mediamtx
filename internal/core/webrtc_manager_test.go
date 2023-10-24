package core

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	rtspurl "github.com/bluenviron/gortsplib/v4/pkg/url"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/webrtc"
)

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
					"  all_others:\n"

			case "internal":
				conf = "paths:\n" +
					"  all_others:\n" +
					"    readUser: myuser\n" +
					"    readPass: mypass\n"

			case "external":
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all_others:\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			var a *testHTTPAuthenticator
			if auth == "external" {
				a = newTestHTTPAuthenticator(t, "rtsp", "publish")
			}

			medi := &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			v := gortsplib.TransportTCP
			source := gortsplib.Client{
				Transport: &v,
			}
			err := source.StartRecording(
				"rtsp://testpublisher:testpass@localhost:8554/teststream?param=value",
				&description.Session{Medias: []*description.Media{medi}})
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

			go func() {
				time.Sleep(500 * time.Millisecond)

				err := source.WritePacketRTP(medi, &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{5, 3},
				})
				require.NoError(t, err)
			}()

			u, err := url.Parse(ur)
			require.NoError(t, err)

			c := &webrtc.WHIPClient{
				HTTPClient: hc,
				URL:        u,
			}

			tracks, err := c.Read(context.Background())
			require.NoError(t, err)
			defer checkClose(t, c.Close)

			pkt, err := tracks[0].ReadRTP()
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
				Payload: []byte{5, 3},
			}, pkt)
		})
	}
}

func TestWebRTCReadNotFound(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all_others:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	iceServers, err := webrtc.WHIPOptionsICEServers(context.Background(), hc, "http://localhost:8889/stream/whep")
	require.NoError(t, err)

	pc, err := pwebrtc.NewPeerConnection(pwebrtc.Configuration{
		ICEServers: iceServers,
	})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	_, err = pc.AddTransceiverFromKind(pwebrtc.RTPCodecTypeVideo)
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
					"  all_others:\n"

			case "internal":
				conf = "paths:\n" +
					"  all_others:\n" +
					"    publishUser: myuser\n" +
					"    publishPass: mypass\n"

			case "external":
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all_others:\n"
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

			su, err := url.Parse(ur)
			require.NoError(t, err)

			s := &webrtc.WHIPClient{
				HTTPClient: hc,
				URL:        su,
			}

			tracks, err := s.Publish(context.Background(), testMediaH264.Formats[0], nil)
			require.NoError(t, err)
			defer checkClose(t, s.Close)

			err = tracks[0].WriteRTP(&rtp.Packet{
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

			time.Sleep(200 * time.Millisecond)

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

			u, err := rtspurl.Parse("rtsp://testreader:testpass@127.0.0.1:8554/teststream?param=value")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			desc, _, err := c.Describe(u)
			require.NoError(t, err)

			var forma *format.H264
			medi := desc.FindFormat(&forma)

			_, err = c.Setup(desc.BaseURL, medi, 0, 0)
			require.NoError(t, err)

			received := make(chan struct{})

			c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
				require.Equal(t, []byte{3}, pkt.Payload)
				close(received)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			err = tracks[0].WriteRTP(&rtp.Packet{
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
