package core

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/webrtc"
)

func TestWebRTCSource(t *testing.T) {
	state := 0

	api, err := webrtc.NewAPI(webrtc.APIConf{})
	require.NoError(t, err)

	pc := &webrtc.PeerConnection{
		API:     api,
		Publish: true,
	}
	err = pc.Start()
	require.NoError(t, err)
	defer pc.Close()

	tracks, err := pc.SetupOutgoingTracks(
		&format.VP8{
			PayloadTyp: 96,
		},
		&format.Opus{
			PayloadTyp: 111,
			IsStereo:   true,
		},
	)
	require.NoError(t, err)

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch state {
			case 0:
				require.Equal(t, http.MethodOptions, r.Method)
				require.Equal(t, "/my/resource", r.URL.Path)

				w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
				w.WriteHeader(http.StatusNoContent)

			case 1:
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/my/resource", r.URL.Path)
				require.Equal(t, "application/sdp", r.Header.Get("Content-Type"))

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				offer := whipOffer(body)

				answer, err := pc.CreateFullAnswer(context.Background(), offer)
				require.NoError(t, err)

				w.Header().Set("Content-Type", "application/sdp")
				w.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
				w.Header().Set("ETag", "test_etag")
				w.Header().Set("Location", "/my/resource/sessionid")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(answer.SDP))

				go func() {
					err = pc.WaitUntilConnected(context.Background())
					require.NoError(t, err)

					err = tracks[0].WriteRTP(&rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
						},
						Payload: []byte{5, 1},
					})
					require.NoError(t, err)

					err = tracks[1].WriteRTP(&rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    97,
							SequenceNumber: 1123,
							Timestamp:      45343,
							SSRC:           563423,
						},
						Payload: []byte{5, 2},
					})
					require.NoError(t, err)
				}()

			default:
				require.Equal(t, "/my/resource/sessionid", r.URL.Path)

				switch r.Method {
				case http.MethodPatch:
					w.WriteHeader(http.StatusNoContent)

				case http.MethodDelete:
					w.WriteHeader(http.StatusOK)

				default:
					t.Errorf("should not happen")
				}
			}
			state++
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5555")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	p, ok := newInstance("paths:\n" +
		"  proxied:\n" +
		"    source: whep://localhost:5555/my/resource\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	c := gortsplib.Client{}

	u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	desc, _, err := c.Describe(u)
	require.NoError(t, err)

	var forma *format.VP8
	medi := desc.FindFormat(&forma)

	_, err = c.Setup(desc.BaseURL, medi, 0, 0)
	require.NoError(t, err)

	received := make(chan struct{})

	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		require.Equal(t, []byte{5, 3}, pkt.Payload)
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
		Payload: []byte{5, 3},
	})
	require.NoError(t, err)

	<-received
}
