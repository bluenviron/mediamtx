package core

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/stretchr/testify/require"
)

type testHTTPAuthenticator struct {
	*http.Server
}

func (ts *testHTTPAuthenticator) initialize(t *testing.T, protocol string, action string) {
	firstReceived := false

	ts.Server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/auth", r.URL.Path)

			var in struct {
				IP       string `json:"ip"`
				User     string `json:"user"`
				Password string `json:"password"`
				Path     string `json:"path"`
				Protocol string `json:"protocol"`
				ID       string `json:"id"`
				Action   string `json:"action"`
				Query    string `json:"query"`
			}
			err := json.NewDecoder(r.Body).Decode(&in)
			require.NoError(t, err)

			var user string
			if action == "publish" {
				user = "testpublisher"
			} else {
				user = "testreader"
			}

			if in.IP != "127.0.0.1" ||
				in.User != user ||
				in.Password != "testpass" ||
				in.Path != "teststream" ||
				in.Protocol != protocol ||
				(firstReceived && in.ID == "") ||
				in.Action != action ||
				(in.Query != "user=testreader&pass=testpass&param=value" &&
					in.Query != "user=testpublisher&pass=testpass&param=value" &&
					in.Query != "param=value") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			firstReceived = true
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:9120")
	require.NoError(t, err)

	go ts.Server.Serve(ln)
}

func (ts *testHTTPAuthenticator) close() {
	ts.Server.Shutdown(context.Background())
}

func TestAuthSha256(t *testing.T) {
	err := doAuthentication(
		"",
		conf.AuthMethods{headers.AuthBasic},
		&conf.Path{
			PublishUser: conf.Credential("sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ="),
			PublishPass: conf.Credential("sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w="),
		},
		defs.PathAccessRequest{
			Name:        "mypath",
			Query:       "",
			Publish:     true,
			SkipAuth:    false,
			IP:          net.ParseIP("127.0.0.1"),
			User:        "testuser",
			Pass:        "testpass",
			Proto:       defs.AuthProtocolRTSP,
			ID:          nil,
			RTSPRequest: nil,
			RTSPBaseURL: nil,
			RTSPNonce:   "",
		},
	)
	require.NoError(t, err)
}

func TestAuthArgon2(t *testing.T) {
	err := doAuthentication(
		"",
		conf.AuthMethods{headers.AuthBasic},
		&conf.Path{
			PublishUser: conf.Credential(
				"argon2:$argon2id$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$Ux/LWeTgJQPyfMMJo1myR64+o8rALHoPmlE1i/TR+58"),
			PublishPass: conf.Credential(
				"argon2:$argon2i$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$/mrZ42TiTv1mcPnpMUera5oi0SFYbbyueAbdx5sUvWo"),
		},
		defs.PathAccessRequest{
			Name:        "mypath",
			Query:       "",
			Publish:     true,
			SkipAuth:    false,
			IP:          net.ParseIP("127.0.0.1"),
			User:        "testuser",
			Pass:        "testpass",
			Proto:       defs.AuthProtocolRTSP,
			ID:          nil,
			RTSPRequest: nil,
			RTSPBaseURL: nil,
			RTSPNonce:   "",
		},
	)
	require.NoError(t, err)
}

func TestAuthExternal(t *testing.T) {
	au := &testHTTPAuthenticator{}
	au.initialize(t, "rtsp", "publish")
	defer au.close()

	err := doAuthentication(
		"http://127.0.0.1:9120/auth",
		conf.AuthMethods{headers.AuthBasic},
		&conf.Path{},
		defs.PathAccessRequest{
			Name:        "teststream",
			Query:       "param=value",
			Publish:     true,
			SkipAuth:    false,
			IP:          net.ParseIP("127.0.0.1"),
			User:        "testpublisher",
			Pass:        "testpass",
			Proto:       defs.AuthProtocolRTSP,
			ID:          nil,
			RTSPRequest: nil,
			RTSPBaseURL: nil,
			RTSPNonce:   "",
		},
	)
	require.NoError(t, err)
}
