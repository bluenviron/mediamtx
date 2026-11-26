package core

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"
)

func TestAPIStaticSourceErrorHistory(t *testing.T) {
	var stream *gortsplib.ServerStream
	server := &gortsplib.Server{
		RTSPAddress:    "127.0.0.1:18555",
		UDPRTPAddress:  "127.0.0.1:18002",
		UDPRTCPAddress: "127.0.0.1:18003",
		Handler: &testServer{
			onDescribe: func(_ *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{StatusCode: base.StatusOK}, stream, nil
			},
			onSetup: func(_ *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{StatusCode: base.StatusOK}, stream, nil
			},
			onPlay: func(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{StatusCode: base.StatusOK}, nil
			},
		},
	}
	require.NoError(t, server.Start())
	defer server.Close()

	// The server accepts PLAY but sends no RTP. The source must time out and retry.
	stream = &gortsplib.ServerStream{
		Server: server,
		Desc: &description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{PayloadTyp: 96, PacketizationMode: 1}},
		}}},
	}
	require.NoError(t, stream.Initialize())
	defer stream.Close()

	start := time.Now()
	instance, ok := newInstance(t, "api: yes\napiAddress: 127.0.0.1:19997\n"+
		"rtsp: no\nrtmp: no\nhls: no\nwebrtc: no\nsrt: no\nmoq: no\nreadTimeout: 2s\n"+
		"paths:\n  camera:\n    source: rtsp://127.0.0.1:18555/camera\n    rtspTransport: udp\n")
	require.True(t, ok)
	defer instance.Close()

	type snapshot struct {
		State            string
		LastError        string
		LastErrorMessage string
		LastErrorTime    *time.Time
		ErrorCount       *uint64
	}

	client := &http.Client{Timeout: time.Second}
	readSource := func() snapshot {
		var source snapshot
		httpRequest(t, client, http.MethodGet, "http://127.0.0.1:19997/v3/paths/static-sources/get/camera", nil, &source)
		return source
	}

	initial := readSource()
	require.NotNil(t, initial.ErrorCount)
	require.Zero(t, *initial.ErrorCount)
	require.Empty(t, initial.LastErrorMessage)
	require.Nil(t, initial.LastErrorTime)

	var failed snapshot
	require.Eventually(t, func() bool {
		failed = readSource()
		return failed.State == "error"
	}, 10*time.Second, 50*time.Millisecond)
	require.Equal(t, "UDP timeout", failed.LastError)
	require.Equal(t, failed.LastError, failed.LastErrorMessage)
	require.NotNil(t, failed.LastErrorTime)
	require.True(t, failed.LastErrorTime.After(start))
	require.EqualValues(t, 1, *failed.ErrorCount)

	require.Eventually(t, func() bool {
		next := readSource()
		return next.State == "running" && next.LastError == "" &&
			next.LastErrorMessage == failed.LastErrorMessage &&
			next.LastErrorTime != nil && next.LastErrorTime.Equal(*failed.LastErrorTime) &&
			next.ErrorCount != nil && *next.ErrorCount == 1
	}, 10*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		next := readSource()
		return next.State == "error" && next.LastError == "UDP timeout" &&
			next.LastErrorMessage == next.LastError &&
			next.LastErrorTime != nil && next.LastErrorTime.After(*failed.LastErrorTime) &&
			next.ErrorCount != nil && *next.ErrorCount == 2
	}, 10*time.Second, 50*time.Millisecond)
}
