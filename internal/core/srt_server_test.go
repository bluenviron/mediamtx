package core

import (
	"bufio"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"
)

func TestSRTServer(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	conf := srt.DefaultConfig()
	address, err := conf.UnmarshalURL("srt://localhost:8890?streamid=publish:mypath")
	require.NoError(t, err)

	err = conf.Validate()
	require.NoError(t, err)

	publisher, err := srt.Dial("srt", address, conf)
	require.NoError(t, err)
	defer publisher.Close()

	track := &mpegts.Track{
		Codec: &mpegts.CodecH264{},
	}

	bw := bufio.NewWriter(publisher)
	w := mpegts.NewWriter(bw, []*mpegts.Track{track})
	require.NoError(t, err)

	err = w.WriteH26x(track, 0, 0, true, [][]byte{
		{ // SPS
			0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
			0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
			0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
			0x20,
		},
		{ // PPS
			0x08, 0x06, 0x07, 0x08,
		},
		{ // IDR
			0x05, 1,
		},
	})
	require.NoError(t, err)

	err = bw.Flush()
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	conf = srt.DefaultConfig()
	address, err = conf.UnmarshalURL("srt://localhost:8890?streamid=read:mypath")
	require.NoError(t, err)

	err = conf.Validate()
	require.NoError(t, err)

	reader, err := srt.Dial("srt", address, conf)
	require.NoError(t, err)
	defer reader.Close()

	err = w.WriteH26x(track, 2*90000, 1*90000, true, [][]byte{
		{ // IDR
			0x05, 2,
		},
	})
	require.NoError(t, err)

	err = bw.Flush()
	require.NoError(t, err)

	r, err := mpegts.NewReader(reader)
	require.NoError(t, err)

	require.Equal(t, []*mpegts.Track{{
		PID:   256,
		Codec: &mpegts.CodecH264{},
	}}, r.Tracks())

	received := false

	r.OnDataH26x(r.Tracks()[0], func(pts int64, dts int64, au [][]byte) error {
		require.Equal(t, int64(0), pts)
		require.Equal(t, int64(0), dts)
		require.Equal(t, [][]byte{
			{ // SPS
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
				0x20,
			},
			{ // PPS
				0x08, 0x06, 0x07, 0x08,
			},
			{ // IDR
				0x05, 1,
			},
		}, au)
		received = true
		return nil
	})

	for {
		err = r.Read()
		require.NoError(t, err)
		if received {
			break
		}
	}
}
