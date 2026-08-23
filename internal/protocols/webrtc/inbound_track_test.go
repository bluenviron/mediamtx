package webrtc

import (
	"fmt"
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

// TestPeerConnectionReadRegistersRTXForVideoCodecs makes sure an incoming
// video track's negotiated SDP actually carries an RTX payload type,
// regardless of which of incomingVideoCodecs' codec families the publisher
// is using. Registering the NACK RTCP interceptor alone (webrtc.ConfigureNack,
// see Start()) isn't enough for pion to answer a publisher's retransmission
// offer with one -- incomingVideoCodecs also needs an explicit RTX codec per
// codec, or a real publisher's own do-retransmission support has nothing to
// negotiate against.
func TestPeerConnectionReadRegistersRTXForVideoCodecs(t *testing.T) {
	for _, ca := range []struct {
		name        string
		mimeType    string
		sdpFmtpLine string
		payloadType uint8
		rtxPT       uint8
	}{
		{"av1", webrtc.MimeTypeAV1, "profile=1", 96, 109},
		{"vp9", webrtc.MimeTypeVP9, "profile-id=0", 101, 126},
		{"vp8", webrtc.MimeTypeVP8, "", 102, 127},
		{"h265", webrtc.MimeTypeH265, "level-id=93;profile-id=2;tier-flag=0;tx-mode=SRST", 103, 35},
		{"h264", webrtc.MimeTypeH264, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f", 106, 107},
	} {
		t.Run(ca.name, func(t *testing.T) {
			// The publisher side needs to offer RTX itself for there to be anything
			// for our answer to match -- exactly what a real GStreamer webrtcbin/
			// whipclientsink publisher does automatically once do-nack is set (which
			// it is, by default). A publisher that only offers the plain codec, with
			// no RTX capability of its own, wouldn't exercise this fix at all: pion's
			// answer only ever includes payload types present in the offer.
			var pubMediaEngine webrtc.MediaEngine
			err := pubMediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    ca.mimeType,
					ClockRate:   90000,
					SDPFmtpLine: ca.sdpFmtpLine,
				},
				PayloadType: webrtc.PayloadType(ca.payloadType),
			}, webrtc.RTPCodecTypeVideo)
			require.NoError(t, err)
			err = pubMediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    webrtc.MimeTypeRTX,
					ClockRate:   90000,
					SDPFmtpLine: fmt.Sprintf("apt=%d", ca.payloadType),
				},
				PayloadType: webrtc.PayloadType(ca.rtxPT),
			}, webrtc.RTPCodecTypeVideo)
			require.NoError(t, err)

			var pubInterceptorRegistry interceptor.Registry
			err = webrtc.ConfigureNack(&pubMediaEngine, &pubInterceptorRegistry)
			require.NoError(t, err)

			api := webrtc.NewAPI(
				webrtc.WithMediaEngine(&pubMediaEngine),
				webrtc.WithInterceptorRegistry(&pubInterceptorRegistry),
			)

			pub, err := api.NewPeerConnection(webrtc.Configuration{})
			require.NoError(t, err)
			defer pub.Close() //nolint:errcheck

			videoTrack, err := webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{
					MimeType:    ca.mimeType,
					ClockRate:   90000,
					SDPFmtpLine: ca.sdpFmtpLine,
				},
				"video", "publisher",
			)
			require.NoError(t, err)

			_, err = pub.AddTrack(videoTrack)
			require.NoError(t, err)

			reader := &PeerConnection{
				LocalRandomUDP:    true,
				IPsFromInterfaces: true,
				Publish:           false,
				Log:               test.NilLogger,
			}
			err = reader.Start()
			require.NoError(t, err)
			defer reader.Close()

			offer, err := pub.CreateOffer(nil)
			require.NoError(t, err)

			err = pub.SetLocalDescription(offer)
			require.NoError(t, err)

			answer, err := reader.CreateFullAnswer(&offer, false)
			require.NoError(t, err)

			var s sdp.SessionDescription
			err = s.Unmarshal([]byte(answer.SDP))
			require.NoError(t, err)

			require.Len(t, s.MediaDescriptions, 1)
			videoMedia := s.MediaDescriptions[0]
			require.Equal(t, "video", videoMedia.MediaName.Media)

			expectedFmtp := fmt.Sprintf("%d apt=%d", ca.rtxPT, ca.payloadType)
			foundFmtp := false
			for _, attr := range videoMedia.Attributes {
				if attr.Key == "fmtp" && attr.Value == expectedFmtp {
					foundFmtp = true
				}
			}
			require.True(t, foundFmtp,
				"answer should offer an RTX payload type (apt=%d) for the negotiated %s track, got SDP:\n%s",
				ca.payloadType, ca.name, answer.SDP)

			expectedRtpmap := fmt.Sprintf("%d rtx/90000", ca.rtxPT)
			foundRTXRtpmap := false
			for _, attr := range videoMedia.Attributes {
				if attr.Key == "rtpmap" && attr.Value == expectedRtpmap {
					foundRTXRtpmap = true
				}
			}
			require.True(t, foundRTXRtpmap, "answer should declare rtpmap for the RTX payload type, got SDP:\n%s", answer.SDP)
		})
	}
}
