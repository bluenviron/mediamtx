package core

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/webrtcpc"
)

func TestWebRTCSource(t *testing.T) {
	state := 0

	api, err := webrtcNewAPI(nil, nil, nil)
	require.NoError(t, err)

	pc, err := webrtcpc.New(nil, api, nilLogger{})
	require.NoError(t, err)
	defer pc.Close()

	outgoingTrack1, err := webrtc.NewTrackLocalStaticRTP(
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

	outgoingTrack2, err := webrtc.NewTrackLocalStaticRTP(
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

				err = pc.SetRemoteDescription(*offer)
				require.NoError(t, err)

				answer, err := pc.CreateAnswer(nil)
				require.NoError(t, err)

				err = pc.SetLocalDescription(answer)
				require.NoError(t, err)

				err = pc.WaitGatheringDone(context.Background())
				require.NoError(t, err)

				w.Header().Set("Content-Type", "application/sdp")
				w.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
				w.Header().Set("E-Tag", "test_etag")
				w.Header().Set("Location", "/my/resource/sessionid")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(pc.LocalDescription().SDP))

				go func() {
					<-pc.Connected()

					err = outgoingTrack1.WriteRTP(&rtp.Packet{
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
							PayloadType:    97,
							SequenceNumber: 1123,
							Timestamp:      45343,
							SSRC:           563423,
						},
						Payload: []byte{2},
					})
					require.NoError(t, err)
				}()

			default:
				t.Errorf("should not happen since there should not be additional candidates")
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

	err = outgoingTrack1.WriteRTP(&rtp.Packet{
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
}
