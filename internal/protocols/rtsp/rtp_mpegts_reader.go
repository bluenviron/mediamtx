package rtsp

import (
	"bytes"
	"io"
	"sync"

	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/counterdumper"
)

// rtpMPEGTSReader provides an io.Reader interface over RTP packets
// containing MPEG-TS data (RFC 2250).
type rtpMPEGTSReader struct {
	packetsLost *counterdumper.CounterDumper

	mu      sync.Mutex
	cond    *sync.Cond
	buffer  *bytes.Buffer
	lastSeq uint16
	seqInit bool
	closed  bool
	err     error
}

func newRTPMPEGTSReader(packetsLost *counterdumper.CounterDumper) *rtpMPEGTSReader {
	r := &rtpMPEGTSReader{
		packetsLost: packetsLost,
		buffer:      &bytes.Buffer{},
	}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *rtpMPEGTSReader) push(pkt *rtp.Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	if r.seqInit {
		expected := r.lastSeq + 1
		if pkt.SequenceNumber != expected {
			// Calculate lost packets, handling wraparound
			var lost uint64
			if pkt.SequenceNumber > expected {
				lost = uint64(pkt.SequenceNumber - expected)
			} else {
				// Sequence number wrapped around
				lost = uint64(0xFFFF - expected + pkt.SequenceNumber + 1)
			}
			if r.packetsLost != nil {
				r.packetsLost.Add(lost)
			}
		}
	}
	r.lastSeq = pkt.SequenceNumber
	r.seqInit = true

	r.buffer.Write(pkt.Payload)
	r.cond.Signal()
}

func (r *rtpMPEGTSReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for r.buffer.Len() == 0 && !r.closed && r.err == nil {
		r.cond.Wait()
	}

	if r.err != nil {
		return 0, r.err
	}

	if r.closed && r.buffer.Len() == 0 {
		return 0, io.EOF
	}

	return r.buffer.Read(p)
}

func (r *rtpMPEGTSReader) close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	r.cond.Broadcast()
}

func (r *rtpMPEGTSReader) setError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
	r.cond.Broadcast()
}
