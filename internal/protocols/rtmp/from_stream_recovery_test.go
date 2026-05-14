package rtmp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// captureLogger collects warn-level log lines so the recovery tests can assert
// the DTS extractor reset message was emitted.
type captureLogger struct {
	mu   sync.Mutex
	logs []string
}

func (c *captureLogger) Log(level logger.Level, format string, args ...any) {
	if level != logger.Warn {
		return
	}
	c.mu.Lock()
	c.logs = append(c.logs, fmt.Sprintf(format, args...))
	c.mu.Unlock()
}

func (c *captureLogger) waitFor(t *testing.T, substr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, l := range c.logs {
			if strings.Contains(l, substr) {
				c.mu.Unlock()
				return
			}
		}
		c.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t.Fatalf("expected warn log containing %q, got %v", substr, c.logs)
}

// runRecoveryScenario sets up a real RTMP server <-> client pair so that
// FromStream's writer can flush packets, then runs writeUnits and waits for
// the expected warn message. The client only consumes track metadata; the
// individual frames are simply drained by the underlying TCP socket.
func runRecoveryScenario(
	t *testing.T,
	medias []*description.Media,
	writeUnits func(medias []*description.Media, sub *stream.SubStream),
	expectedWarn string,
) {
	t.Helper()

	strm := &stream.Stream{
		Desc:              &description.Session{Medias: medias},
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	subStream := &stream.SubStream{Stream: strm, UseRTPPackets: false}
	require.NoError(t, subStream.Initialize())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	clientDone := make(chan struct{})

	go func() {
		defer close(clientDone)

		u, perr := url.Parse("rtmp://" + ln.Addr().String() + "/stream")
		require.NoError(t, perr)

		c := &gortmplib.Client{URL: u}
		require.NoError(t, c.Initialize(context.Background()))

		r := &gortmplib.Reader{Conn: c}
		require.NoError(t, r.Initialize())
	}()

	nconn, err := ln.Accept()
	require.NoError(t, err)
	defer nconn.Close()

	conn := &gortmplib.ServerConn{RW: nconn}
	require.NoError(t, conn.Initialize())
	require.NoError(t, conn.Accept())

	cl := &captureLogger{}
	r := &stream.Reader{Parent: cl}

	require.NoError(t, FromStream(strm.Desc, r, conn, nconn, 5*time.Second))

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	writeUnits(medias, subStream)

	cl.waitFor(t, expectedWarn)

	// Reader should still be alive: a permanent failure would have closed
	// the channel with a real error before now.
	select {
	case err := <-r.Error():
		t.Fatalf("reader unexpectedly terminated: %v", err)
	default:
	}

	<-clientDone // best-effort wait so test cleanup doesn't race
}

func TestFromStreamH264DTSExtractorRecovery(t *testing.T) {
	medias := []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}}

	idr := unit.PayloadH264{test.FormatH264.SPS, test.FormatH264.PPS, {0x65}}

	writeUnits := func(medias []*description.Media, sub *stream.SubStream) {
		// Prime the extractor with a valid IDR.
		sub.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
			PTS:     90000,
			Payload: idr,
		})
		// Trigger DTS error (PTS goes backwards). The fix logs a warn,
		// resets the extractor and returns nil so the reader survives.
		sub.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
			PTS:     0,
			Payload: idr,
		})
		// Subsequent IDR re-primes the extractor.
		sub.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
			PTS:     180000,
			Payload: idr,
		})
	}

	runRecoveryScenario(t, medias, writeUnits, "H264 DTS extractor reset")
}
