package httpp

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestHandlerTracker(t *testing.T) {
	requestReceived := make(chan struct{})

	s := &Server{
		Address:      "localhost:4667",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Parent:       test.NilLogger,
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			close(requestReceived)
			time.Sleep(1 * time.Second)
		}),
	}
	err := s.Initialize()
	require.NoError(t, err)

	go func() {
		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		hc := &http.Client{Transport: tr}

		_, err2 := hc.Get("http://localhost:4667/test") //nolint:bodyclose
		require.Error(t, err2)
	}()

	<-requestReceived

	beforeClose := time.Now()

	s.Close()

	require.Greater(t, time.Since(beforeClose), 1*time.Second)
}
