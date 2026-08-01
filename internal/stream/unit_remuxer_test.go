package stream_test

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestUnitRemuxer(t *testing.T) {
	for _, ca := range []string{
		"av1",
		"h265",
		"h264",
		"mpeg4video",
	} {
		t.Run(ca, func(t *testing.T) {
			var forma format.Format
			var inputPayload unit.Payload
			var expectedPayload unit.Payload

			switch ca {
			case "av1":
				forma = &format.AV1{}
				inputPayload = unit.PayloadAV1{
					{0x10},       // Temporal Delimiter
					{0x08, 0x01}, // Sequence Header
					{0x10},       // Temporal Delimiter
				}
				expectedPayload = unit.PayloadAV1{{0x08, 0x01}}

			case "h265":
				vps := []byte{0x40, 0x01, 0x0c}
				sps := []byte{0x42, 0x01, 0x01}
				pps := []byte{0x44, 0x01, 0xc1}

				forma = &format.H265{
					VPS: vps,
					SPS: sps,
					PPS: pps,
				}
				inputPayload = unit.PayloadH265{
					vps,
					sps,
					pps,
					{0x46, 0x01}, // AUD
					{0x26, 0x01}, // IDR
				}
				expectedPayload = unit.PayloadH265{
					vps,
					sps,
					pps,
					{0x26, 0x01},
				}

			case "h264":
				sps := []byte{0x67, 0x64, 0x00, 0x20}
				pps := []byte{0x68, 0xee, 0x3c, 0x80}

				forma = &format.H264{
					PacketizationMode: 1,
					SPS:               sps,
					PPS:               pps,
				}
				inputPayload = unit.PayloadH264{
					sps,
					pps,
					{0x09, 0xf0}, // AUD
					{0x65, 0x01}, // IDR
				}
				expectedPayload = unit.PayloadH264{
					sps,
					pps,
					{0x65, 0x01},
				}

			case "mpeg4video":
				config := []byte{0x00, 0x00, 0x01, 0xb0, 0x01}
				frame := []byte{0x00, 0x00, 0x01, 0xb3, 0x11, 0x22}

				forma = &format.MPEG4Video{Config: config}
				inputPayload = unit.PayloadMPEG4Video(frame)
				expectedPayload = unit.PayloadMPEG4Video(append(append([]byte{}, config...), frame...))
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
			recv := make(chan *unit.Unit, 1)

			r.OnData(media, forma, func(u *unit.Unit) error {
				recv <- u
				return nil
			})

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			subStream.WriteUnit(media, forma, &unit.Unit{
				PTS:     90000,
				Payload: inputPayload,
			})

			received := <-recv
			require.Equal(t, expectedPayload, received.Payload)
		})
	}
}
