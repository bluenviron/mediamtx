package mpegtstimedec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNegativeDiff(t *testing.T) {
	d := New(64523434)

	ts := d.Decode(64523434 - 90000)
	require.Equal(t, -1*time.Second, ts)

	ts = d.Decode(64523434)
	require.Equal(t, time.Duration(0), ts)

	ts = d.Decode(64523434 + 90000*2)
	require.Equal(t, 2*time.Second, ts)

	ts = d.Decode(64523434 + 90000)
	require.Equal(t, 1*time.Second, ts)
}

func TestOverflow(t *testing.T) {
	d := New(0x1FFFFFFFF - 20)

	i := int64(0x1FFFFFFFF - 20)
	secs := time.Duration(0)
	const stride = 150
	lim := int64(uint64(0x1FFFFFFFF - (stride * 90000)))

	for n := 0; n < 100; n++ {
		// overflow
		i += 90000 * stride
		secs += stride
		ts := d.Decode(i)
		require.Equal(t, secs*time.Second, ts)

		// reach 2^32 slowly
		secs += stride
		i += 90000 * stride
		for ; i < lim; i += 90000 * stride {
			ts = d.Decode(i)
			require.Equal(t, secs*time.Second, ts)
			secs += stride
		}
	}
}

func TestOverflowAndBack(t *testing.T) {
	d := New(0x1FFFFFFFF - 90000 + 1)

	ts := d.Decode(0x1FFFFFFFF - 90000 + 1)
	require.Equal(t, time.Duration(0), ts)

	ts = d.Decode(90000)
	require.Equal(t, 2*time.Second, ts)

	ts = d.Decode(0x1FFFFFFFF - 90000 + 1)
	require.Equal(t, time.Duration(0), ts)

	ts = d.Decode(0x1FFFFFFFF - 90000*2 + 1)
	require.Equal(t, -1*time.Second, ts)

	ts = d.Decode(0x1FFFFFFFF - 90000 + 1)
	require.Equal(t, time.Duration(0), ts)

	ts = d.Decode(90000)
	require.Equal(t, 2*time.Second, ts)
}
