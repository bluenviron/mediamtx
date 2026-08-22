package webrtc

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

// TestPeerConnectionReadRegistersRTXForH264 makes sure an incoming H264 video
// track's negotiated SDP actually carries an RTX payload type. Registering
// the NACK RTCP interceptor alone (webrtc.ConfigureNack, see Start()) isn't
// enough for pion to answer a publisher's retransmission offer with one --
// incomingVideoCodecs also needs an explicit RTX codec, or a real publisher's
// own do-retransmission support has nothing to negotiate against.
func TestPeerConnectionReadRegistersRTXForH264(t *testing.T) {
	// The publisher side needs to offer RTX itself for there to be anything
	// for our answer to match -- exactly what a real GStreamer webrtcbin/
	// whipclientsink publisher does automatically once do-nack is set (which
	// it is, by default). A publisher that only offers plain H264, with no
	// RTX capability of its own, wouldn't exercise this fix at all: pion's
	// answer only ever includes payload types present in the offer.
	var pubMediaEngine webrtc.MediaEngine
	err := pubMediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 106,
	}, webrtc.RTPCodecTypeVideo)
	require.NoError(t, err)
	err = pubMediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeRTX,
			ClockRate:   90000,
			SDPFmtpLine: "apt=106",
		},
		PayloadType: 107,
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
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
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

	rtxPayloadType := ""
	for _, attr := range videoMedia.Attributes {
		if attr.Key == "fmtp" && attr.Value == "107 apt=106" {
			rtxPayloadType = "107"
		}
	}
	require.NotEmpty(t, rtxPayloadType, "answer should offer an RTX payload type (apt=106) for the negotiated H264 track, got SDP:\n%s", answer.SDP)

	foundRTXRtpmap := false
	for _, attr := range videoMedia.Attributes {
		if attr.Key == "rtpmap" && attr.Value == rtxPayloadType+" rtx/90000" {
			foundRTXRtpmap = true
		}
	}
	require.True(t, foundRTXRtpmap, "answer should declare rtpmap for the RTX payload type, got SDP:\n%s", answer.SDP)
}
