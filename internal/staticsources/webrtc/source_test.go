package webrtc

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/test"
)

func whipOffer(body []byte) *pwebrtc.SessionDescription {
	return &pwebrtc.SessionDescription{
		Type: pwebrtc.SDPTypeOffer,
		SDP:  string(body),
	}
}

func TestSource(t *testing.T) {
	api, err := webrtc.NewAPI(webrtc.APIConf{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
	})
	require.NoError(t, err)

	pc := &webrtc.PeerConnection{
		API:     api,
		Publish: true,
		Log:     test.NilLogger{},
	}
	err = pc.Start()
	require.NoError(t, err)
	defer pc.Close()

	tracks, err := pc.SetupOutgoingTracks(
		nil,
		&format.Opus{
			PayloadTyp: 111,
			IsStereo:   true,
		},
	)
	require.NoError(t, err)

	state := 0

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
							PayloadType:    111,
							SequenceNumber: 1123,
							Timestamp:      45343,
							SSRC:           563424,
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

	ln, err := net.Listen("tcp", "localhost:9003")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	te := test.NewSourceTester(
		func(p defs.StaticSourceParent) defs.StaticSource {
			return &Source{
				ResolvedSource: "whep://localhost:9003/my/resource",
				ReadTimeout:    conf.StringDuration(10 * time.Second),
				Parent:         p,
			}
		},
		&conf.Path{},
	)
	defer te.Close()

	<-te.Unit
}
