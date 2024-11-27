package core

import (
	"bufio"
	"net"
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/stretchr/testify/require"
)

func TestPathAutoDeletion(t *testing.T) {
	for _, ca := range []string{"describe", "setup"} {
		t.Run(ca, func(t *testing.T) {
			p, ok := newInstance("paths:\n" +
				"  all_others:\n")
			require.Equal(t, true, ok)
			defer p.Close()

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" {
					u, err := base.ParseURL("rtsp://localhost:8554/mypath")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Describe,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Unmarshal(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusNotFound, res.StatusCode)
				} else {
					u, err := base.ParseURL("rtsp://localhost:8554/mypath/trackID=0")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Setup,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
							"Transport": headers.Transport{
								Mode: func() *headers.TransportMode {
									v := headers.TransportModePlay
									return &v
								}(),
								Delivery: func() *headers.TransportDelivery {
									v := headers.TransportDeliveryUnicast
									return &v
								}(),
								Protocol:    headers.TransportProtocolUDP,
								ClientPorts: &[2]int{35466, 35467},
							}.Marshal(),
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Unmarshal(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusNotFound, res.StatusCode)
				}
			}()

			data, err := p.pathManager.APIPathsList()
			require.NoError(t, err)

			require.Empty(t, data.Items)
		})
	}
}
