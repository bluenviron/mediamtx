package core

import (
	"encoding/binary"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/ntp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func TestPathNTP64Extension(t *testing.T) {
	for _, ca := range []string{"teststream", "relay"} {
		t.Run(ca, func(t *testing.T) {
			p, ok := newInstance(t, "api: yes\n"+
				"paths:\n"+
				"  teststream:\n"+
				"    useAbsoluteTimestamp: yes\n"+
				"  relay:\n"+
				"    source: rtsp://localhost:8554/teststream\n"+
				"    sourceOnDemand: yes\n"+
				"    useAbsoluteTimestamp: yes\n")
			require.Equal(t, true, ok)
			defer p.Close()

			medi := &description.Media{
				Type:             description.MediaTypeVideo,
				HeaderExtensions: []description.HeaderExtension{{ID: 1, URI: description.NTP64ExtensionURI}},
				Formats:          []format.Format{test.FormatH264},
			}

			source := gortsplib.Client{}
			err := source.StartRecording("rtsp://localhost:8554/teststream",
				&description.Session{Medias: []*description.Media{medi}})
			require.NoError(t, err)
			defer source.Close()

			u, err := base.ParseURL("rtsp://localhost:8554/" + ca)
			require.NoError(t, err)

			reader := gortsplib.Client{Scheme: u.Scheme, Host: u.Host}
			err = reader.Start()
			require.NoError(t, err)
			defer reader.Close()

			desc, _, err := reader.Describe(u)
			require.NoError(t, err)
			require.Equal(t, medi.HeaderExtensions, desc.Medias[0].HeaderExtensions)

			err = reader.SetupAll(desc.BaseURL, desc.Medias)
			require.NoError(t, err)

			recv := make(chan time.Time, 1)
			reader.OnPacketRTP(desc.Medias[0], desc.Medias[0].Formats[0], func(pkt *rtp.Packet) {
				v, _ := reader.PacketNTP(desc.Medias[0], pkt)
				recv <- v
			})

			_, err = reader.Play(nil)
			require.NoError(t, err)

			if ca == "relay" {
				// the relay must read teststream before the packet is sent.
				hc := &http.Client{Transport: &http.Transport{}}
				defer hc.CloseIdleConnections()

				for range 100 {
					var out struct {
						ReaderCount int `json:"readerCount"`
					}
					httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/teststream", nil, &out)
					if out.ReaderCount != 0 {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
			}

			stamp := time.Now()
			ext := make([]byte, 8)
			binary.BigEndian.PutUint64(ext, ntp.Encode(stamp))

			pkt := &rtp.Packet{Version: 2, Marker: true, PayloadType: 96, Payload: []byte{5, 1, 2, 3, 4}}
			err = pkt.SetExtension(1, ext)
			require.NoError(t, err)

			// a single stamped packet is sent before any RTCP sender report.
			err = source.WritePacketRTP(medi, pkt)
			require.NoError(t, err)

			select {
			case v := <-recv:
				// a sender report of the server may arrive first; its 90 kHz RTP clock limits the precision.
				require.WithinDuration(t, stamp, v, time.Millisecond)
			case <-time.After(5 * time.Second):
				t.Error("first packet not received")
			}
		})
	}
}
