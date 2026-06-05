package httpp3

import (
	"crypto/tls"
	"io"
	"net/http"
	"testing"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	// test that Server can be initialized and reached by a simple GET request
	s := &Server{
		Address: "127.0.0.1:18443",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Parent: test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		},
	}
	defer tr.Close() //nolint:errcheck
	hc := &http.Client{Transport: tr}

	res, err := hc.Get("https://127.0.0.1:18443/")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	_, err = io.ReadAll(res.Body)
	require.NoError(t, err)
}
