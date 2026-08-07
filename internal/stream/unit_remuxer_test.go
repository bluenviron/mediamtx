package stream_test

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
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

func setupUnitRemuxerMergeTest(
	t *testing.T,
	media *description.Media,
	forma format.Format,
) (*stream.SubStream, chan *unit.Unit) {
	desc := &description.Session{Medias: []*description.Media{media}}

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	t.Cleanup(strm.Close)

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	r := &stream.Reader{}
	recv := make(chan *unit.Unit, 8)
	r.OnData(media, forma, func(u *unit.Unit) error {
		recv <- u
		return nil
	})
	strm.AddReader(r)
	t.Cleanup(func() { strm.RemoveReader(r) })

	return subStream, recv
}

// TestUnitRemuxerH264SEIMerge verifies that a non-VCL H264 access unit (e.g.
// the standalone SEI access unit some DJI drones send ahead of every
// picture) is held back and merged into the next access unit that shares
// its PTS and carries a picture, instead of being emitted as its own
// (incomplete) frame.
func TestUnitRemuxerH264SEIMerge(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20}
	pps := []byte{0x68, 0xee, 0x3c, 0x80}
	sei := []byte{0x06, 0x01, 0x02}
	slice := []byte{0x01, 0xaa, 0xbb}
	aud := []byte{0x09, 0xf0}

	forma := &format.H264{
		PacketizationMode: 1,
		SPS:               sps,
		PPS:               pps,
	}

	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}

	t.Run("sei then slice at same pts merges", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH264{sei},
		})
		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH264{slice},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())

		u2 := <-recv
		require.Equal(t, unit.PayloadH264{sei, slice}, u2.Payload)
	})

	t.Run("sei then slice at different pts does not merge", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH264{sei},
		})
		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     93000,
			Payload: unit.PayloadH264{slice},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())

		u2 := <-recv
		require.Equal(t, unit.PayloadH264{slice}, u2.Payload)
	})

	t.Run("aud only still yields nil", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH264{aud},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())
	})
}

// TestUnitRemuxerH265SEIMerge verifies that a non-VCL H265 access unit (e.g.
// a standalone prefix-SEI access unit) is held back and merged into the next
// access unit that shares its PTS and carries a picture, instead of being
// emitted as its own (incomplete) frame. It also verifies that a solitary
// suffix SEI, which belongs to a picture that would already have been
// emitted, is dropped rather than held back.
func TestUnitRemuxerH265SEIMerge(t *testing.T) {
	vps := []byte{0x40, 0x01, 0x0c}
	sps := []byte{0x42, 0x01, 0x01}
	pps := []byte{0x44, 0x01, 0xc1}
	prefixSEI := []byte{0x4e, 0x01, 0x02} // PREFIX_SEI_NUT
	suffixSEI := []byte{0x50, 0x01, 0x02} // SUFFIX_SEI_NUT
	slice := []byte{0x02, 0xaa, 0xbb}     // non-IDR slice
	idr := []byte{0x26, 0x01}             // IDR_W_RADL
	aud := []byte{0x46, 0x01}

	forma := &format.H265{
		VPS: vps,
		SPS: sps,
		PPS: pps,
	}

	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}

	t.Run("prefix sei then slice at same pts merges", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{prefixSEI},
		})
		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{slice},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())

		u2 := <-recv
		require.Equal(t, unit.PayloadH265{prefixSEI, slice}, u2.Payload)
	})

	t.Run("prefix sei then slice at different pts does not merge", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{prefixSEI},
		})
		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     93000,
			Payload: unit.PayloadH265{slice},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())

		u2 := <-recv
		require.Equal(t, unit.PayloadH265{slice}, u2.Payload)
	})

	t.Run("prefix sei then IDR at same pts merges with parameters prepended", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{prefixSEI},
		})
		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{idr},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())

		u2 := <-recv
		require.Equal(t, unit.PayloadH265{vps, sps, pps, prefixSEI, idr}, u2.Payload)
	})

	t.Run("solitary suffix sei is dropped, not merged", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{suffixSEI},
		})
		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{slice},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())

		u2 := <-recv
		require.Equal(t, unit.PayloadH265{slice}, u2.Payload)
	})

	t.Run("aud only still yields nil", func(t *testing.T) {
		subStream, recv := setupUnitRemuxerMergeTest(t, media, forma)

		subStream.WriteUnit(media, forma, &unit.Unit{
			PTS:     90000,
			Payload: unit.PayloadH265{aud},
		})

		u1 := <-recv
		require.True(t, u1.NilPayload())
	})
}
