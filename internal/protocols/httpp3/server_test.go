package httpp3_test

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/httpp3"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestServer(t *testing.T) {
	// test that Server can be initialized and reached by a simple GET request
	s := &httpp3.Server{
		Address:            "127.0.0.1:18443",
		UDPReadBufferSize:  10000,
		UDPWriteBufferSize: 20000,
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

func TestServerWebTransportSettings(t *testing.T) {
	const (
		settingsInitialMaxData        = 0x2b61
		settingsInitialMaxStreamsUni  = 0x2b64
		settingsInitialMaxStreamsBidi = 0x2b65
	)

	s := &httpp3.Server{
		Address:            "127.0.0.1:18444",
		EnableWebTransport: true,
		Handler:            http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		Parent:             test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()

	qconn, err := quic.DialAddr(
		ctx,
		"127.0.0.1:18444",
		&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
			NextProtos:         []string{http3.NextProtoH3},
		},
		&quic.Config{EnableDatagrams: true},
	)
	require.NoError(t, err)
	defer qconn.CloseWithError(0, "") //nolint:errcheck

	tr := &http3.Transport{EnableDatagrams: true}
	cc := tr.NewClientConn(qconn)

	select {
	case <-cc.ReceivedSettings():
	case <-ctx.Done():
		t.Fatal("settings not received")
	}

	settings := cc.Settings()
	require.NotZero(t, settings.Other[settingsInitialMaxStreamsUni])
	require.NotZero(t, settings.Other[settingsInitialMaxStreamsBidi])
	require.NotZero(t, settings.Other[settingsInitialMaxData])
}
