package formatprocessor

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/stretchr/testify/require"
)

func TestAV1KeyFrameWarning(t *testing.T) { //nolint:dupl
	forma := &formats.AV1{
		PayloadTyp: 96,
	}

	w := &testLogWriter{recv: make(chan string, 1)}
	p, err := New(1472, forma, true, w)
	require.NoError(t, err)

	ntp := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	err = p.Process(&UnitAV1{
		BaseUnit: BaseUnit{
			NTP: ntp,
		},
		OBUs: [][]byte{
			{0x01},
		},
	}, false)
	require.NoError(t, err)

	ntp = ntp.Add(30 * time.Second)
	err = p.Process(&UnitAV1{
		BaseUnit: BaseUnit{
			NTP: ntp,
		},
		OBUs: [][]byte{
			{0x01},
		},
	}, false)
	require.NoError(t, err)

	logl := <-w.recv
	require.Equal(t, "no AV1 key frames received in 10s, stream can't be decoded", logl)
}
