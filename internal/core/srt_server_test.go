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
	for _, ca := range []string{
		"no passphrase",
		"publish passphrase",
		"read passphrase",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "paths:\n" +
				"  all_others:\n"

			switch ca {
			case "publish passphrase":
				conf += "    srtPublishPassphrase: 123456789abcde"

			case "read passphrase":
				conf += "    srtReadPassphrase: 123456789abcde"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			u := "srt://localhost:8890?streamid=publish:mypath"
			if ca == "publish passphrase" {
				u += "&passphrase=123456789abcde"
			}

			srtConf := srt.DefaultConfig()
			address, err := srtConf.UnmarshalURL(u)
			require.NoError(t, err)

			err = srtConf.Validate()
			require.NoError(t, err)

			publisher, err := srt.Dial("srt", address, srtConf)
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

			u = "srt://localhost:8890?streamid=read:mypath"
			if ca == "read passphrase" {
				u += "&passphrase=123456789abcde"
			}

			srtConf = srt.DefaultConfig()
			address, err = srtConf.UnmarshalURL(u)
			require.NoError(t, err)

			err = srtConf.Validate()
			require.NoError(t, err)

			reader, err := srt.Dial("srt", address, srtConf)
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
		})
	}
}
