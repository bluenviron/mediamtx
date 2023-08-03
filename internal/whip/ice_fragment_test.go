package whip

import (
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func stringPtr(v string) *string {
	return &v
}

func uint16Ptr(v uint16) *uint16 {
	return &v
}

var iceFragmentCases = []struct {
	name       string
	offer      string
	candidates []*webrtc.ICECandidateInit
	enc        string
}{
	{
		"a",
		"v=0\n" +
			"o=- 8429658789122714282 1690995382 IN IP4 0.0.0.0\n" +
			"s=-\n" +
			"t=0 0\n" +
			"a=fingerprint:sha-256 EA:05:9D:04:8F:56:41:92:3E:D5:2B:55:03:" +
			"1B:5A:2C:3D:D8:B3:FB:1B:D9:F7:1F:DA:77:0E:B9:E0:3D:B6:FF\n" +
			"a=extmap-allow-mixed\n" +
			"a=group:BUNDLE 0\n" +
			"m=video 9 UDP/TLS/RTP/SAVPF 96 97 98 99 100 101 102 121 127 120 125 107 108 109 123 118 45 46 116\n" +
			"c=IN IP4 0.0.0.0\n" +
			"a=setup:actpass\n" +
			"a=mid:0\n" +
			"a=ice-ufrag:tUQMzoQAVLzlvBys\n" +
			"a=ice-pwd:pimyGfJcjjRwvUjnmGOODSjtIxyDljQj\n" +
			"a=rtcp-mux\n" +
			"a=rtcp-rsize\n" +
			"a=rtpmap:96 VP8/90000\n" +
			"a=rtcp-fb:96 goog-remb \n" +
			"a=rtcp-fb:96 ccm fir\n" +
			"a=rtcp-fb:96 nack \n" +
			"a=rtcp-fb:96 nack pli\n" +
			"a=rtcp-fb:96 nack \n" +
			"a=rtcp-fb:96 nack pli\n" +
			"a=rtcp-fb:96 transport-cc \n" +
			"a=rtpmap:97 rtx/90000\n" +
			"a=fmtp:97 apt=96\n" +
			"a=rtcp-fb:97 nack \n" +
			"a=rtcp-fb:97 nack pli\n" +
			"a=rtcp-fb:97 transport-cc \n" +
			"a=rtpmap:98 VP9/90000\n" +
			"a=fmtp:98 profile-id=0\n" +
			"a=rtcp-fb:98 goog-remb \n" +
			"a=rtcp-fb:98 ccm fir\n" +
			"a=rtcp-fb:98 nack \n" +
			"a=rtcp-fb:98 nack pli\n" +
			"a=rtcp-fb:98 nack \n" +
			"a=rtcp-fb:98 nack pli\n" +
			"a=rtcp-fb:98 transport-cc \n" +
			"a=rtpmap:99 rtx/90000\n" +
			"a=fmtp:99 apt=98\n" +
			"a=rtcp-fb:99 nack \n" +
			"a=rtcp-fb:99 nack pli\n" +
			"a=rtcp-fb:99 transport-cc \n" +
			"a=rtpmap:100 VP9/90000\n" +
			"a=fmtp:100 profile-id=1\n" +
			"a=rtcp-fb:100 goog-remb \n" +
			"a=rtcp-fb:100 ccm fir\n" +
			"a=rtcp-fb:100 nack \n" +
			"a=rtcp-fb:100 nack pli\n" +
			"a=rtcp-fb:100 nack \n" +
			"a=rtcp-fb:100 nack pli\n" +
			"a=rtcp-fb:100 transport-cc \n" +
			"a=rtpmap:101 rtx/90000\n" +
			"a=fmtp:101 apt=100\n" +
			"a=rtcp-fb:101 nack \n" +
			"a=rtcp-fb:101 nack pli\n" +
			"a=rtcp-fb:101 transport-cc \n" +
			"a=rtpmap:102 H264/90000\n" +
			"a=fmtp:102 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f\n" +
			"a=rtcp-fb:102 goog-remb \n" +
			"a=rtcp-fb:102 ccm fir\n" +
			"a=rtcp-fb:102 nack \n" +
			"a=rtcp-fb:102 nack pli\n" +
			"a=rtcp-fb:102 nack \n" +
			"a=rtcp-fb:102 nack pli\n" +
			"a=rtcp-fb:102 transport-cc \n" +
			"a=rtpmap:121 rtx/90000\n" +
			"a=fmtp:121 apt=102\n" +
			"a=rtcp-fb:121 nack \n" +
			"a=rtcp-fb:121 nack pli\n" +
			"a=rtcp-fb:121 transport-cc \n" +
			"a=rtpmap:127 H264/90000\n" +
			"a=fmtp:127 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f\n" +
			"a=rtcp-fb:127 goog-remb \n" +
			"a=rtcp-fb:127 ccm fir\n" +
			"a=rtcp-fb:127 nack \n" +
			"a=rtcp-fb:127 nack pli\n" +
			"a=rtcp-fb:127 nack \n" +
			"a=rtcp-fb:127 nack pli\n" +
			"a=rtcp-fb:127 transport-cc \n" +
			"a=rtpmap:120 rtx/90000\n" +
			"a=fmtp:120 apt=127\n" +
			"a=rtcp-fb:120 nack \n" +
			"a=rtcp-fb:120 nack pli\n" +
			"a=rtcp-fb:120 transport-cc \n" +
			"a=rtpmap:125 H264/90000\n" +
			"a=fmtp:125 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f\n" +
			"a=rtcp-fb:125 goog-remb \n" +
			"a=rtcp-fb:125 ccm fir\n" +
			"a=rtcp-fb:125 nack \n" +
			"a=rtcp-fb:125 nack pli\n" +
			"a=rtcp-fb:125 nack \n" +
			"a=rtcp-fb:125 nack pli\n" +
			"a=rtcp-fb:125 transport-cc \n" +
			"a=rtpmap:107 rtx/90000\n" +
			"a=fmtp:107 apt=125\n" +
			"a=rtcp-fb:107 nack \n" +
			"a=rtcp-fb:107 nack pli\n" +
			"a=rtcp-fb:107 transport-cc \n" +
			"a=rtpmap:108 H264/90000\n" +
			"a=fmtp:108 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f\n" +
			"a=rtcp-fb:108 goog-remb \n" +
			"a=rtcp-fb:108 ccm fir\n" +
			"a=rtcp-fb:108 nack \n" +
			"a=rtcp-fb:108 nack pli\n" +
			"a=rtcp-fb:108 nack \n" +
			"a=rtcp-fb:108 nack pli\n" +
			"a=rtcp-fb:108 transport-cc \n" +
			"a=rtpmap:109 rtx/90000\n" +
			"a=fmtp:109 apt=108\n" +
			"a=rtcp-fb:109 nack \n" +
			"a=rtcp-fb:109 nack pli\n" +
			"a=rtcp-fb:109 transport-cc \n" +
			"a=rtpmap:123 H264/90000\n" +
			"a=fmtp:123 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032\n" +
			"a=rtcp-fb:123 goog-remb \n" +
			"a=rtcp-fb:123 ccm fir\n" +
			"a=rtcp-fb:123 nack \n" +
			"a=rtcp-fb:123 nack pli\n" +
			"a=rtcp-fb:123 nack \n" +
			"a=rtcp-fb:123 nack pli\n" +
			"a=rtcp-fb:123 transport-cc \n" +
			"a=rtpmap:118 rtx/90000\n" +
			"a=fmtp:118 apt=123\n" +
			"a=rtcp-fb:118 nack \n" +
			"a=rtcp-fb:118 nack pli\n" +
			"a=rtcp-fb:118 transport-cc \n" +
			"a=rtpmap:45 AV1/90000\n" +
			"a=rtcp-fb:45 goog-remb \n" +
			"a=rtcp-fb:45 ccm fir\n" +
			"a=rtcp-fb:45 nack \n" +
			"a=rtcp-fb:45 nack pli\n" +
			"a=rtcp-fb:45 nack \n" +
			"a=rtcp-fb:45 nack pli\n" +
			"a=rtcp-fb:45 transport-cc \n" +
			"a=rtpmap:46 rtx/90000\n" +
			"a=fmtp:46 apt=45\n" +
			"a=rtcp-fb:46 nack \n" +
			"a=rtcp-fb:46 nack pli\n" +
			"a=rtcp-fb:46 transport-cc \n" +
			"a=rtpmap:116 ulpfec/90000\n" +
			"a=rtcp-fb:116 nack \n" +
			"a=rtcp-fb:116 nack pli\n" +
			"a=rtcp-fb:116 transport-cc \n" +
			"a=extmap:1 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01\n" +
			"a=ssrc:3421396091 cname:BmFVQDtOlcBwXZCl\n" +
			"a=ssrc:3421396091 msid:BmFVQDtOlcBwXZCl CLgunVCazXXKLyEx\n" +
			"a=ssrc:3421396091 mslabel:BmFVQDtOlcBwXZCl\n" +
			"a=ssrc:3421396091 label:CLgunVCazXXKLyEx\n" +
			"a=msid:BmFVQDtOlcBwXZCl CLgunVCazXXKLyEx\n" +
			"a=sendrecv\n",
		[]*webrtc.ICECandidateInit{{
			Candidate:     "3628911098 1 udp 2130706431 192.168.3.218 49462 typ host",
			SDPMid:        stringPtr("0"),
			SDPMLineIndex: uint16Ptr(0),
		}},
		"a=ice-ufrag:tUQMzoQAVLzlvBys\r\n" +
			"a=ice-pwd:pimyGfJcjjRwvUjnmGOODSjtIxyDljQj\r\n" +
			"m=video 9 UDP/TLS/RTP/SAVPF 96 97 98 99 100 101 102 121 127 120 125 107 108 109 123 118 45 46 116\r\n" +
			"a=mid:0\r\n" +
			"a=candidate:3628911098 1 udp 2130706431 192.168.3.218 49462 typ host\r\n",
	},
}

func TestICEFragmentUnmarshal(t *testing.T) {
	for _, ca := range iceFragmentCases {
		t.Run(ca.name, func(t *testing.T) {
			candidates, err := ICEFragmentUnmarshal([]byte(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.candidates, candidates)
		})
	}
}

func TestICEFragmentMarshal(t *testing.T) {
	for _, ca := range iceFragmentCases {
		t.Run(ca.name, func(t *testing.T) {
			byts, err := ICEFragmentMarshal(ca.offer, ca.candidates)
			require.NoError(t, err)
			require.Equal(t, ca.enc, string(byts))
		})
	}
}
