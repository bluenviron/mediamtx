package stream_test

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestRTPEncoder(t *testing.T) {
	for _, ca := range casesDecodeEncode {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{{
				Formats: []format.Format{ca.format},
			}}}

			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
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

			r.OnData(desc.Medias[0], ca.format, func(u *unit.Unit) error {
				for i := range min(len(u.RTPPackets), len(ca.encoded)) {
					u.RTPPackets[i].Timestamp = ca.encoded[i].Timestamp
					u.RTPPackets[i].SequenceNumber = ca.encoded[i].SequenceNumber
					u.RTPPackets[i].SSRC = ca.encoded[i].SSRC
				}
				require.Equal(t, ca.encoded, u.RTPPackets)
				close(recv)
				return nil
			})

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			subStream.WriteUnit(desc.Medias[0], ca.format, &unit.Unit{
				Payload: ca.decoded,
			})

			<-recv
		})
	}
}
