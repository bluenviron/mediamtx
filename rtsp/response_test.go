package rtsp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var casesResponse = []struct {
	name string
	byts []byte
	res  *Response
}{
	{
		"ok",
		[]byte("RTSP/1.0 200 OK\r\n" +
			"CSeq: 1\r\n" +
			"Public: DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE\r\n" +
			"\r\n",
		),
		&Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":   "1",
				"Public": "DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE",
			},
		},
	},
	{
		"ok with content",
		[]byte("RTSP/1.0 200 OK\r\n" +
			"CSeq: 2\r\n" +
			"Content-Base: rtsp://example.com/media.mp4\r\n" +
			"Content-Length: 444\r\n" +
			"Content-Type: application/sdp\r\n" +
			"\r\n" +
			"m=video 0 RTP/AVP 96\n" +
			"a=control:streamid=0\n" +
			"a=range:npt=0-7.741000\n" +
			"a=length:npt=7.741000\n" +
			"a=rtpmap:96 MP4V-ES/5544\n" +
			"a=mimetype:string;\"video/MP4V-ES\"\n" +
			"a=AvgBitRate:integer;304018\n" +
			"a=StreamName:string;\"hinted video track\"\n" +
			"m=audio 0 RTP/AVP 97\n" +
			"a=control:streamid=1\n" +
			"a=range:npt=0-7.712000\n" +
			"a=length:npt=7.712000\n" +
			"a=rtpmap:97 mpeg4-generic/32000/2\n" +
			"a=mimetype:string;\"audio/mpeg4-generic\"\n" +
			"a=AvgBitRate:integer;65790\n" +
			"a=StreamName:string;\"hinted audio track\"\n",
		),
		&Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"Content-Base":   "rtsp://example.com/media.mp4",
				"Content-Length": "444",
				"Content-Type":   "application/sdp",
				"CSeq":           "2",
			},
			Content: []byte("m=video 0 RTP/AVP 96\n" +
				"a=control:streamid=0\n" +
				"a=range:npt=0-7.741000\n" +
				"a=length:npt=7.741000\n" +
				"a=rtpmap:96 MP4V-ES/5544\n" +
				"a=mimetype:string;\"video/MP4V-ES\"\n" +
				"a=AvgBitRate:integer;304018\n" +
				"a=StreamName:string;\"hinted video track\"\n" +
				"m=audio 0 RTP/AVP 97\n" +
				"a=control:streamid=1\n" +
				"a=range:npt=0-7.712000\n" +
				"a=length:npt=7.712000\n" +
				"a=rtpmap:97 mpeg4-generic/32000/2\n" +
				"a=mimetype:string;\"audio/mpeg4-generic\"\n" +
				"a=AvgBitRate:integer;65790\n" +
				"a=StreamName:string;\"hinted audio track\"\n",
			),
		},
	},
}

func TestResponseDecode(t *testing.T) {
	for _, c := range casesResponse {
		t.Run(c.name, func(t *testing.T) {
			res, err := responseDecode(bytes.NewBuffer(c.byts))
			require.NoError(t, err)
			require.Equal(t, c.res, res)
		})
	}
}

func TestResponseEncode(t *testing.T) {
	for _, c := range casesResponse {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := responseEncode(&buf, c.res)
			require.NoError(t, err)
			require.Equal(t, c.byts, buf.Bytes())
		})
	}
}
