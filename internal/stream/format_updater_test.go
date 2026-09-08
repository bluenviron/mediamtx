package stream_test

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestFormatUpdater(t *testing.T) {
	for _, ca := range []string{"h265", "h264", "mpeg4video"} {
		t.Run(ca, func(t *testing.T) {
			var forma format.Format
			var u *unit.Unit

			switch ca {
			case "h265":
				vps := []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xfe}
				sps := []byte{0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x04}
				pps := []byte{0x44, 0x01, 0xc1, 0x73, 0xd1, 0x8a}

				forma = &format.H265{
					VPS: []byte{0x40, 0x01, 0x0c},
					SPS: []byte{0x42, 0x01, 0x01},
					PPS: []byte{0x44, 0x01, 0xc1},
				}

				u = &unit.Unit{
					PTS: 90000,
					Payload: unit.PayloadH265{
						vps,                      // New VPS
						sps,                      // New SPS
						pps,                      // New PPS
						{0x26, 0x01, 0x01, 0x02}, // IDR
					},
				}

			case "h264":
				sps := []byte{
					0x67, 0x64, 0x00, 0x20, 0xac, 0xd9, 0x40, 0x78,
					0x02, 0x27, 0xe5, 0x9a, 0x80, 0x80, 0x80, 0xa0,
				}
				pps := []byte{0x08, 0x07, 0x08, 0x09}

				forma = &format.H264{
					PacketizationMode: 1,
					SPS:               []byte{0x67, 0x42, 0xc0, 0x28},
					PPS:               []byte{0x08, 0x06},
				}

				u = &unit.Unit{
					PTS: 90000,
					Payload: unit.PayloadH264{
						sps,    // New SPS
						pps,    // New PPS
						{5, 1}, // IDR
					},
				}

			case "mpeg4video":
				config := []byte{
					0x00, 0x00, 0x01, 0xb0, // Visual Object Sequence Start
					0x02, 0x00, 0x00, 0x01, 0xb5, 0x8a,
					0x14, 0x00, 0x00, 0x01, 0x00,
				}

				forma = &format.MPEG4Video{
					Config: []byte{0x00, 0x00, 0x01, 0xb0, 0x01},
				}

				frame := make([]byte, 0, len(config)+20)
				frame = append(frame, config...)
				frame = append(frame, []byte{0x00, 0x00, 0x01, 0xb3}...) // Group of VOP
				frame = append(frame, []byte{0x01, 0x02, 0x03, 0x04}...)

				u = &unit.Unit{
					PTS:     90000,
					Payload: unit.PayloadMPEG4Video(frame),
				}
			}

			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}

			desc := &description.Session{Medias: []*description.Media{media}}

			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			r := &stream.Reader{}
			recv := make(chan struct{})

			r.OnData(media, forma, func(_ *unit.Unit) error {
				close(recv)
				return nil
			})

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			subStream.WriteUnit(media, forma, u)

			<-recv

			outDesc := strm.OutDescCopy()

			// Verify that format parameters were updated
			switch ca {
			case "h265":
				require.Equal(t, &format.H265{
					VPS: []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xfe},
					SPS: []byte{0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x04},
					PPS: []byte{0x44, 0x01, 0xc1, 0x73, 0xd1, 0x8a},
				}, outDesc.Medias[0].Formats[0])

			case "h264":
				require.Equal(t, &format.H264{
					SPS: []byte{
						0x67, 0x64, 0x00, 0x20, 0xac, 0xd9, 0x40, 0x78,
						0x02, 0x27, 0xe5, 0x9a, 0x80, 0x80, 0x80, 0xa0,
					},
					PPS:               []byte{0x08, 0x07, 0x08, 0x09},
					PacketizationMode: 1,
				}, outDesc.Medias[0].Formats[0])

			case "mpeg4video":
				require.Equal(t, &format.MPEG4Video{
					Config: []byte{
						0x00, 0x00, 0x01, 0xb0,
						0x02, 0x00, 0x00, 0x01, 0xb5, 0x8a,
						0x14, 0x00, 0x00, 0x01, 0x00,
					},
				}, outDesc.Medias[0].Formats[0])
			}
		})
	}
}
