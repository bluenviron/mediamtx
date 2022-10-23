package mpegts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeDecoderNegativeDiff(t *testing.T) {
	d := NewTimeDecoder()

	i := int64(0)
	pts := d.Decode(i)
	require.Equal(t, time.Duration(0), pts)

	i += 90000 * 2
	pts = d.Decode(i)
	require.Equal(t, 2*time.Second, pts)

	i -= 90000 * 1
	pts = d.Decode(i)
	require.Equal(t, 1*time.Second, pts)

	i += 90000 * 2
	pts = d.Decode(i)
	require.Equal(t, 3*time.Second, pts)
}

func TestTimeDecoderOverflow(t *testing.T) {
	d := NewTimeDecoder()

	i := int64(0x1FFFFFFFF - 20)
	secs := time.Duration(0)
	pts := d.Decode(i)
	require.Equal(t, time.Duration(0), pts)

	const stride = 150
	lim := int64(uint64(0x1FFFFFFFF - (stride * 90000)))

	for n := 0; n < 100; n++ {
		// overflow
		i += 90000 * stride
		secs += stride
		pts = d.Decode(i)
		require.Equal(t, secs*time.Second, pts)

		// reach 2^32 slowly
		secs += stride
		i += 90000 * stride
		for ; i < lim; i += 90000 * stride {
			pts = d.Decode(i)
			require.Equal(t, secs*time.Second, pts)
			secs += stride
		}
	}
}

func TestTimeDecoderOverflowAndBack(t *testing.T) {
	d := NewTimeDecoder()

	pts := d.Decode(0x1FFFFFFFF - 90000 + 1)
	require.Equal(t, time.Duration(0), pts)

	pts = d.Decode(90000)
	require.Equal(t, 2*time.Second, pts)

	pts = d.Decode(0x1FFFFFFFF - 90000 + 1)
	require.Equal(t, time.Duration(0), pts)

	pts = d.Decode(0x1FFFFFFFF - 90000*2 + 1)
	require.Equal(t, -1*time.Second, pts)

	pts = d.Decode(0x1FFFFFFFF - 90000 + 1)
	require.Equal(t, time.Duration(0), pts)

	pts = d.Decode(90000)
	require.Equal(t, 2*time.Second, pts)
}
