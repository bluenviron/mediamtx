package core_test

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/core"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestAPIRTSPConnsKick(t *testing.T) {
	cert := test.CreateTempFile(t, test.TLSCertPub)
	key := test.CreateTempFile(t, test.TLSCertKey)

	for _, scheme := range []string{"rtsp", "rtsps"} {
		t.Run(scheme, func(t *testing.T) {
			config := "api: yes\nrtspTransports: [tcp]\nrtmp: no\nhls: no\nwebrtc: no\nsrt: no\nmoq: no\n"
			addr := "localhost:8554"
			client := gortsplib.Client{Scheme: scheme}
			if scheme == "rtsps" {
				config += "rtspEncryption: strict\nrtspServerCert: " + cert + "\nrtspServerKey: " + key + "\n"
				addr = "localhost:8322"
				client.TLSConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
			}
			client.Host = addr

			server, ok := core.New([]string{test.CreateTempFile(t, []byte(config))})
			require.True(t, ok)
			defer server.Close()

			u, err := base.ParseURL(scheme + "://" + addr + "/mystream")
			require.NoError(t, err)
			require.NoError(t, client.Start())
			defer client.Close()
			_, err = client.Options(u)
			require.NoError(t, err)

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr, Timeout: 2 * time.Second}
			apiURL := "http://localhost:9997/v3/" + scheme + "/conns"
			list := func() defs.APIRTSPConnsList {
				res, err2 := hc.Get(apiURL + "/list")
				require.NoError(t, err2)
				defer res.Body.Close()
				require.Equal(t, http.StatusOK, res.StatusCode)
				var out defs.APIRTSPConnsList
				require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
				return out
			}
			kick := func(id string, status int) {
				res, err2 := hc.Post(apiURL+"/kick/"+id, "", nil)
				require.NoError(t, err2)
				defer res.Body.Close()
				require.Equal(t, status, res.StatusCode)
			}

			conns := list()
			require.Len(t, conns.Items, 1)
			require.Nil(t, conns.Items[0].Session)
			id := conns.Items[0].ID.String()

			kick("invalid-uuid", http.StatusBadRequest)
			kick(id, http.StatusOK)
			require.Empty(t, list().Items)
			kick(id, http.StatusNotFound)
			require.ErrorIs(t, client.Wait(), io.EOF)
		})
	}
}
