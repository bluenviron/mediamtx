package httpp

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
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

		res, err2 := hc.Get("http://localhost:4667/test")
		require.NoError(t, err2)
		defer res.Body.Close()
	}()

	<-requestReceived

	beforeClose := time.Now()

	s.Close()

	require.Greater(t, time.Since(beforeClose), 1*time.Second)
}
