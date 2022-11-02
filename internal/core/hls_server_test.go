package core

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type testHTTPAuthenticator struct {
	action string

	s *http.Server
}

func newTestHTTPAuthenticator(action string) (*testHTTPAuthenticator, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:9120")
	if err != nil {
		return nil, err
	}

	ts := &testHTTPAuthenticator{
		action: action,
	}

	router := gin.New()
	router.POST("/auth", ts.onAuth)

	ts.s = &http.Server{Handler: router}
	go ts.s.Serve(ln)

	return ts, nil
}

func (ts *testHTTPAuthenticator) close() {
	ts.s.Shutdown(context.Background())
}

func (ts *testHTTPAuthenticator) onAuth(ctx *gin.Context) {
	var in struct {
		IP       string `json:"ip"`
		User     string `json:"user"`
		Password string `json:"password"`
		Path     string `json:"path"`
		Action   string `json:"action"`
		Query    string `json:"query"`
	}
	err := json.NewDecoder(ctx.Request.Body).Decode(&in)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	var user string
	if ts.action == "publish" {
		user = "testpublisher"
	} else {
		user = "testreader"
	}

	if in.IP != "127.0.0.1" ||
		in.User != user ||
		in.Password != "testpass" ||
		in.Path != "teststream" ||
		in.Action != ts.action ||
		(in.Query != "user=testreader&pass=testpass&param=value" &&
			in.Query != "user=testpublisher&pass=testpass&param=value" &&
			in.Query != "param=value") {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
}

func TestHLSServerNotFound(t *testing.T) {
	p, ok := newInstance("")
	require.Equal(t, true, ok)
	defer p.Close()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/stream/", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}
