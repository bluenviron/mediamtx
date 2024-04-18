package webrtc

import (
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestPeerConnectionCloseAfterError(t *testing.T) {
	api, err := NewAPI(APIConf{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
	})
	require.NoError(t, err)

	pc := &PeerConnection{
		API:     api,
		Publish: false,
		Log:     test.NilLogger,
	}
	err = pc.Start()
	require.NoError(t, err)

	_, err = pc.CreatePartialOffer()
	require.NoError(t, err)

	// wait for ICE candidates to be generated
	time.Sleep(500 * time.Millisecond)

	pc.Close()
}
