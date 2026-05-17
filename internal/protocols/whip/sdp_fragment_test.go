package whip

import (
	"testing"

	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/require"
)

var sdpFragmentCases = []struct {
	name string
	enc  string
	dec  *SDPFragment
}{
	{
		"session-wide credentials",
		"a=ice-ufrag:tUQMzoQAVLzlvBys\r\n" +
			"a=ice-pwd:pimyGfJcjjRwvUjnmGOODSjtIxyDljQj\r\n" +
			"m=video 9 UDP/TLS/RTP/SAVPF 96 97 98 99 100 101 102 121 127 120 125 107 108 109 123 118 45 46 116\r\n" +
			"a=mid:0\r\n" +
			"a=candidate:3628911098 1 udp 2130706431 192.168.3.218 49462 typ host\r\n",
		&SDPFragment{
			Attributes: []sdp.Attribute{
				{Key: "ice-ufrag", Value: "tUQMzoQAVLzlvBys"},
				{Key: "ice-pwd", Value: "pimyGfJcjjRwvUjnmGOODSjtIxyDljQj"},
			},
			Medias: []*sdp.MediaDescription{{
				MediaName: sdp.MediaName{
					Media:  "video",
					Port:   sdp.RangedPort{Value: 9},
					Protos: []string{"UDP", "TLS", "RTP", "SAVPF"},
					Formats: []string{
						"96", "97", "98", "99", "100", "101", "102",
						"121", "127", "120", "125", "107", "108", "109", "123", "118", "45", "46", "116",
					},
				},
				Attributes: []sdp.Attribute{
					{Key: "mid", Value: "0"},
					{Key: "candidate", Value: "3628911098 1 udp 2130706431 192.168.3.218 49462 typ host"},
				},
			}},
		},
	},
	{
		"rfc9725 trickle ice patch request",
		"a=group:BUNDLE 0 1\r\n" +
			"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\n" +
			"a=mid:0\r\n" +
			"a=ice-ufrag:EsAw\r\n" +
			"a=ice-pwd:P2uYro0UCOQ4zxjKXaWCBui1\r\n" +
			"a=candidate:1387637174 1 udp 2122260223 192.0.2.1 61764 typ host" +
			" generation 0 ufrag EsAw network-id 1\r\n" +
			"a=candidate:3471623853 1 udp 2122194687 198.51.100.2 61765 typ host" +
			" generation 0 ufrag EsAw network-id 2\r\n" +
			"a=candidate:473322822 1 tcp 1518280447 192.0.2.1 9 typ host tcptype active" +
			" generation 0 ufrag EsAw network-id 1\r\n" +
			"a=candidate:2154773085 1 tcp 1518214911 198.51.100.2 9 typ host tcptype active" +
			" generation 0 ufrag EsAw network-id 2\r\n" +
			"a=end-of-candidates\r\n",
		&SDPFragment{
			Attributes: []sdp.Attribute{
				{Key: "group", Value: "BUNDLE 0 1"},
			},
			Medias: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media:   "audio",
						Port:    sdp.RangedPort{Value: 9},
						Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
						Formats: []string{"111"},
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "0"},
						{Key: "ice-ufrag", Value: "EsAw"},
						{Key: "ice-pwd", Value: "P2uYro0UCOQ4zxjKXaWCBui1"},
						{Key: "candidate", Value: "1387637174 1 udp 2122260223 192.0.2.1 61764 typ host" +
							" generation 0 ufrag EsAw network-id 1"},
						{Key: "candidate", Value: "3471623853 1 udp 2122194687 198.51.100.2 61765 typ host" +
							" generation 0 ufrag EsAw network-id 2"},
						{Key: "candidate", Value: "473322822 1 tcp 1518280447 192.0.2.1 9 typ host tcptype active" +
							" generation 0 ufrag EsAw network-id 1"},
						{Key: "candidate", Value: "2154773085 1 tcp 1518214911 198.51.100.2 9 typ host tcptype active" +
							" generation 0 ufrag EsAw network-id 2"},
						{Key: "end-of-candidates", Value: ""},
					},
				},
			},
		},
	},
	{
		"rfc9725 ice restart patch request",
		"a=ice-options:trickle ice2\r\n" +
			"a=group:BUNDLE 0 1\r\n" +
			"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\n" +
			"a=mid:0\r\n" +
			"a=ice-ufrag:ysXw\r\n" +
			"a=ice-pwd:vw5LmwG4y/e6dPP/zAP9Gp5k\r\n" +
			"a=candidate:1387637174 1 udp 2122260223 192.0.2.1 61764 typ host" +
			" generation 0 ufrag EsAw network-id 1\r\n" +
			"a=candidate:3471623853 1 udp 2122194687 198.51.100.2 61765 typ host" +
			" generation 0 ufrag EsAw network-id 2\r\n" +
			"a=candidate:473322822 1 tcp 1518280447 192.0.2.1 9 typ host tcptype active" +
			" generation 0 ufrag EsAw network-id 1\r\n" +
			"a=candidate:2154773085 1 tcp 1518214911 198.51.100.2 9 typ host tcptype active" +
			" generation 0 ufrag EsAw network-id 2\r\n",
		&SDPFragment{
			Attributes: []sdp.Attribute{
				{Key: "ice-options", Value: "trickle ice2"},
				{Key: "group", Value: "BUNDLE 0 1"},
			},
			Medias: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media:   "audio",
						Port:    sdp.RangedPort{Value: 9},
						Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
						Formats: []string{"111"},
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "0"},
						{Key: "ice-ufrag", Value: "ysXw"},
						{Key: "ice-pwd", Value: "vw5LmwG4y/e6dPP/zAP9Gp5k"},
						{Key: "candidate", Value: "1387637174 1 udp 2122260223 192.0.2.1 61764 typ host" +
							" generation 0 ufrag EsAw network-id 1"},
						{Key: "candidate", Value: "3471623853 1 udp 2122194687 198.51.100.2 61765 typ host" +
							" generation 0 ufrag EsAw network-id 2"},
						{Key: "candidate", Value: "473322822 1 tcp 1518280447 192.0.2.1 9 typ host tcptype active" +
							" generation 0 ufrag EsAw network-id 1"},
						{Key: "candidate", Value: "2154773085 1 tcp 1518214911 198.51.100.2 9 typ host tcptype active" +
							" generation 0 ufrag EsAw network-id 2"},
					},
				},
			},
		},
	},
	{
		"rfc9725 ice restart patch response",
		"a=ice-lite\r\n" +
			"a=ice-options:trickle ice2\r\n" +
			"a=group:BUNDLE 0 1\r\n" +
			"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\n" +
			"a=mid:0\r\n" +
			"a=ice-ufrag:289b31b754eaa438\r\n" +
			"a=ice-pwd:0b66f472495ef0ccac7bda653ab6be49ea13114472a5d10a\r\n" +
			"a=candidate:1 1 udp 2130706431 198.51.100.1 39132 typ host\r\n" +
			"a=end-of-candidates\r\n",
		&SDPFragment{
			Attributes: []sdp.Attribute{
				{Key: "ice-lite", Value: ""},
				{Key: "ice-options", Value: "trickle ice2"},
				{Key: "group", Value: "BUNDLE 0 1"},
			},
			Medias: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media:   "audio",
						Port:    sdp.RangedPort{Value: 9},
						Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
						Formats: []string{"111"},
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "0"},
						{Key: "ice-ufrag", Value: "289b31b754eaa438"},
						{Key: "ice-pwd", Value: "0b66f472495ef0ccac7bda653ab6be49ea13114472a5d10a"},
						{Key: "candidate", Value: "1 1 udp 2130706431 198.51.100.1 39132 typ host"},
						{Key: "end-of-candidates", Value: ""},
					},
				},
			},
		},
	},
}

func TestSDPFragmentUnmarshal(t *testing.T) {
	for _, ca := range sdpFragmentCases {
		t.Run(ca.name, func(t *testing.T) {
			frag := &SDPFragment{}
			err := frag.Unmarshal([]byte(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dec, frag)
		})
	}
}

func TestSDPFragmentMarshal(t *testing.T) {
	for _, ca := range sdpFragmentCases {
		t.Run(ca.name, func(t *testing.T) {
			byts, err := ca.dec.Marshal()
			require.NoError(t, err)
			require.Equal(t, ca.enc, string(byts))
		})
	}
}
