package stream

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/ringbuffer"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func receiptQueueFixture(stamp bool) (*subStreamFormat, *Reader, *unit.Unit) {
	buffer, _ := ringbuffer.New(16)
	r := &Reader{SkipOutboundBytes: true, buffer: buffer}
	if stamp {
		r.TimeNow = time.Now
	}
	sf := &streamFormat{
		inboundBytes: new(atomic.Uint64), outboundBytes: new(atomic.Uint64),
		writeRTSP: func([]*rtp.Packet, time.Time) {},
		onDatas:   map[*Reader]OnDataFunc{r: func(*unit.Unit) error { return nil }},
	}
	return &subStreamFormat{streamFormat: sf}, r, &unit.Unit{}
}

func enqueueAndPull(sf *subStreamFormat, r *Reader, u *unit.Unit) {
	if err := sf.writeUnitInner(u); err != nil {
		panic(err)
	}
	cb, ok := r.buffer.Pull()
	if !ok {
		panic("queue unexpectedly closed")
	}
	if err := cb.(func() error)(); err != nil {
		panic(err)
	}
}

func TestReaderReceiptAllocationCount(t *testing.T) {
	plain, plainReader, u := receiptQueueFixture(false)
	stamped, stampedReader, v := receiptQueueFixture(true)
	plainAllocs := testing.AllocsPerRun(1000, func() { enqueueAndPull(plain, plainReader, u) })
	stampedAllocs := testing.AllocsPerRun(1000, func() { enqueueAndPull(stamped, stampedReader, v) })
	require.Equal(t, plainAllocs, stampedAllocs, "stamping must not add another heap object per queued unit")
	t.Logf("allocations per enqueue+callback: plain=%v stamped=%v", plainAllocs, stampedAllocs)
	plainReader.buffer.Close()
	stampedReader.buffer.Close()
}

func BenchmarkReaderReceiptTimestamp(b *testing.B) {
	for _, stamp := range []bool{false, true} {
		name := "plain"
		if stamp {
			name = "stamped"
		}
		b.Run(name, func(b *testing.B) {
			sf, r, u := receiptQueueFixture(stamp)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				enqueueAndPull(sf, r, u)
			}
			b.StopTimer()
			r.buffer.Close()
		})
	}
}
