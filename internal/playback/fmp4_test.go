package playback

import (
	"io"
	"os"
	"testing"

	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

func writeBenchInit(f io.WriteSeeker) {
	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &fmp4.CodecH264{
					SPS: []byte{
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
						0x20,
					},
					PPS: []byte{0x08},
				},
			},
			{
				ID:        2,
				TimeScale: 90000,
				Codec: &fmp4.CodecMPEG4Audio{
					Config: mpeg4audio.Config{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	err := init.Marshal(f)
	if err != nil {
		panic(err)
	}

	_, err = f.Write([]byte{
		'm', 'o', 'o', 'f', 0x00, 0x00, 0x00, 0x10,
	})
	if err != nil {
		panic(err)
	}
}

func BenchmarkFMP4ReadInit(b *testing.B) {
	f, err := os.CreateTemp(os.TempDir(), "mediamtx-playback-fmp4-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())

	writeBenchInit(f)
	f.Close()

	for n := 0; n < b.N; n++ {
		func() {
			f, err := os.Open(f.Name())
			if err != nil {
				panic(err)
			}
			defer f.Close()

			_, err = fmp4ReadInit(f)
			if err != nil {
				panic(err)
			}
		}()
	}
}
