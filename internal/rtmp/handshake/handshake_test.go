package handshake

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testReadWriter struct {
	ch chan []byte
}

func (rw *testReadWriter) Read(p []byte) (int, error) {
	in := <-rw.ch
	n := copy(p, in)
	return n, nil
}

func (rw *testReadWriter) Write(p []byte) (int, error) {
	rw.ch <- p
	return len(p), nil
}

func TestHandshake(t *testing.T) {
	for _, ca := range []string{"plain", "encrypted"} {
		t.Run(ca, func(t *testing.T) {
			rw := &testReadWriter{ch: make(chan []byte)}
			var serverInKey []byte
			var serverOutKey []byte
			done := make(chan struct{})

			go func() {
				var err error
				serverInKey, serverOutKey, err = DoServer(rw, true)
				require.NoError(t, err)
				close(done)
			}()

			clientInKey, clientOutKey, err := DoClient(rw, ca == "encrypted", true)
			require.NoError(t, err)
			<-done

			if ca == "encrypted" {
				require.NotNil(t, serverInKey)
				require.Equal(t, serverInKey, clientOutKey)
				require.Equal(t, serverOutKey, clientInKey)
			}
		})
	}
}
