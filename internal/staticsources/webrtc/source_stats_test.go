package webrtc

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/test"
)

// a fresh source, before connecting, reports nil stats.
func TestSourceStatsEmpty(t *testing.T) {
	so := &Source{}
	require.Nil(t, so.SourceStats())
}

func TestSourceStats(t *testing.T) {
	outboundTracks := []*webrtc.OutboundTrack{{
		Caps: pwebrtc.RTPCodecCapability{
			MimeType:    "audio/opus",
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
		},
	}}

	pc := &webrtc.PeerConnection{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
		Publish:           true,
		OutboundTracks:    outboundTracks,
		Log:               test.NilLogger,
	}
	err := pc.Start()
	require.NoError(t, err)
	defer pc.Close()

	state := 0

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch state {
			case 0:
				w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
				w.WriteHeader(http.StatusNoContent)

			case 1:
				body, err2 := io.ReadAll(r.Body)
				require.NoError(t, err2)
				offer := whipOffer(body)

				answer, err2 := pc.CreateFullAnswer(offer, false)
				require.NoError(t, err2)

				w.Header().Set("Content-Type", "application/sdp")
				w.Header().Set("ETag", "test_etag")
				w.Header().Set("Location", "/my/resource/sessionid")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(answer.SDP))

				go func() {
					err3 := pc.WaitUntilConnected(10 * time.Second)
					require.NoError(t, err3)

					err3 = outboundTracks[0].WriteRTP(&rtp.Packet{
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
					require.NoError(t, err3)
				}()

			default:
				switch r.Method {
				case http.MethodPatch:
					w.WriteHeader(http.StatusNoContent)

				case http.MethodDelete:
					w.WriteHeader(http.StatusOK)
				}
			}
			state++
		}),
	}

	ln, err := net.Listen("tcp", "localhost:9007")
	require.NoError(t, err)

	go httpServ.Serve(ln)
	defer httpServ.Shutdown(context.Background())

	p := &test.StaticSourceParent{}
	p.Initialize()
	defer p.Close()

	so := &Source{
		ReadTimeout: conf.Duration(10 * time.Second),
		Parent:      p,
	}

	done := make(chan struct{})
	defer func() { <-done }()

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	reloadConf := make(chan *conf.Path)

	go func() {
		so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
			Context:        ctx,
			ResolvedSource: "whep://localhost:9007/my/resource",
			Conf:           &conf.Path{},
			ReloadConf:     reloadConf,
		})
		close(done)
	}()

	<-p.Unit

	// stats are populated asynchronously from RTCP/receiver stats,
	// so poll until at least one inbound packet is accounted for.
	require.Eventually(t, func() bool {
		st := so.SourceStats()
		if st == nil {
			return false
		}
		wst, ok := st.(*defs.WebRTCSourceStats)
		if !ok {
			return false
		}
		return wst.Jitter != nil && wst.PacketsReceived >= 1
	}, 5*time.Second, 50*time.Millisecond)

	// the source must be listening on ReloadConf
	reloadConf <- nil
}
