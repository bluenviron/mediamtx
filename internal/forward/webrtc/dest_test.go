package webrtc_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	forwardwebrtc "github.com/bluenviron/mediamtx/internal/forward/webrtc"
	mtxwebrtc "github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func startWHIPServer(
	t *testing.T,
	expectedBearerToken string,
) (string, <-chan struct{}, <-chan error) {
	pc := &mtxwebrtc.PeerConnection{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
		Log:               test.NilLogger,
	}
	err := pc.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		pc.Close()
	})

	received := make(chan struct{}, 1)
	serverErr := make(chan error, 1)

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expectedBearerToken != "" {
				require.Equal(t, "Bearer "+expectedBearerToken, r.Header.Get("Authorization"))
			}

			switch {
			case r.Method == http.MethodOptions && r.URL.Path == "/stream/whip":
				w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.WriteHeader(http.StatusNoContent)

			case r.Method == http.MethodPost && r.URL.Path == "/stream/whip":
				require.Equal(t, "application/sdp", r.Header.Get("Content-Type"))

				body, err2 := io.ReadAll(r.Body)
				require.NoError(t, err2)

				offer := &pwebrtc.SessionDescription{
					Type: pwebrtc.SDPTypeOffer,
					SDP:  string(body),
				}

				answer, err2 := pc.CreateFullAnswer(offer, false)
				require.NoError(t, err2)

				w.Header().Set("Content-Type", "application/sdp")
				w.Header().Set("Location", "/stream/whip/sessionid")
				w.WriteHeader(http.StatusCreated)
				_, err2 = w.Write([]byte(answer.SDP))
				require.NoError(t, err2)

				go func() {
					err3 := pc.WaitUntilConnected(10 * time.Second)
					if err3 != nil {
						serverErr <- err3
						return
					}

					err3 = pc.GatherInboundTracks(2 * time.Second)
					if err3 != nil {
						serverErr <- err3
						return
					}

					if len(pc.InboundTracks()) != 1 {
						serverErr <- fmt.Errorf("unexpected track count: %d", len(pc.InboundTracks()))
						return
					}

					pc.InboundTracks()[0].OnPacketRTP = func(_ *rtp.Packet) {
						select {
						case received <- struct{}{}:
						default:
						}
					}

					pc.StartReading()
				}()

			case r.Method == http.MethodDelete && r.URL.Path == "/stream/whip/sessionid":
				w.WriteHeader(http.StatusOK)

			default:
				serverErr <- fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusBadRequest)
			}
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go httpServ.Serve(ln)

	t.Cleanup(func() {
		httpServ.Shutdown(context.Background())
	})

	return "whip://" + ln.Addr().String() + "/stream/whip", received, serverErr
}

func TestDest(t *testing.T) {
	const bearerToken = "mytoken"

	destURL, received, serverErr := startWHIPServer(t, bearerToken)

	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	require.NoError(t, subStream.Initialize())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dest := &forwardwebrtc.Dest{
		Stream:      strm,
		Dest:        destURL,
		ReadTimeout: conf.Duration(10 * time.Second),
		BearerToken: bearerToken,
		Parent:      test.NilLogger,
	}

	done := make(chan error, 1)
	go func() {
		done <- dest.Run(ctx)
	}()

	strm.WaitForReaders()
	for i := range 2 {
		subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS:     int64(i) * 2 * 90000,
			Payload: unit.PayloadH264{{5, 2}},
		})
	}

	select {
	case <-received:
	case err := <-serverErr:
		require.NoError(t, err)
	case runErr := <-done:
		t.Fatalf("WHIP destination stopped before forwarding a frame: %v", runErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WHIP frame")
	}

	require.Eventually(t, func() bool {
		return dest.OutboundBytes() > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case runErr := <-done:
		require.Error(t, runErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WHIP destination to stop")
	}
}
