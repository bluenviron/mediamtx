package core

import (
	"bufio"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
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

func TestPathConfigurationHotReload(t *testing.T) {
	// Start MediaMTX with basic configuration
	p, ok := newInstance("api: yes\n" +
		"paths:\n" +
		"  all:\n" +
		"    record: no\n")
	require.Equal(t, true, ok)
	defer p.Close()

	// Set up HTTP client for API calls
	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	// Create a publisher that will use the "all" configuration
	media0 := test.UniqueMediaH264()
	source := gortsplib.Client{}
	err := source.StartRecording(
		"rtsp://localhost:8554/undefined_stream",
		&description.Session{Medias: []*description.Media{media0}})
	require.NoError(t, err)
	defer source.Close()

	// Send some data to establish the stream
	err = source.WritePacketRTP(media0, &rtp.Packet{
		Header: rtp.Header{
			Version:     2,
			PayloadType: 96,
		},
		Payload: []byte{5, 1, 2, 3, 4},
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify the path exists and is using the "all" configuration
	pathData, err := p.pathManager.APIPathsGet("undefined_stream")
	require.NoError(t, err)
	require.Equal(t, "undefined_stream", pathData.Name)
	require.Equal(t, "all", pathData.ConfName)

	// Check the current configuration via API
	var allConfig map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/all", nil, &allConfig)
	require.Equal(t, false, allConfig["record"]) // Should be false from "all" config

	// Add a new specific configuration for "undefined_stream" with record enabled
	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/undefined_stream",
		map[string]interface{}{
			"record": true,
		}, nil)

	// Give the system time to process the configuration change
	time.Sleep(200 * time.Millisecond)

	// Verify the path now uses the new specific configuration
	pathData, err = p.pathManager.APIPathsGet("undefined_stream")
	require.NoError(t, err)
	require.Equal(t, "undefined_stream", pathData.Name)
	require.Equal(t, "undefined_stream", pathData.ConfName) // Should now use the specific config

	// Check the new configuration via API
	var newConfig map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/undefined_stream", nil, &newConfig)
	require.Equal(t, true, newConfig["record"]) // Should be true from new config

	// Verify the stream is still active and working
	err = source.WritePacketRTP(media0, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 2,
		},
		Payload: []byte{5, 1, 2, 3, 4},
	})
	require.NoError(t, err)

	// Verify the path is still ready and functional
	require.Equal(t, true, pathData.Ready)

	// revert configuration
	httpRequest(t, hc, http.MethodDelete, "http://localhost:9997/v3/config/paths/delete/undefined_stream",
		nil, nil)

	// Give the system time to process the configuration change
	time.Sleep(200 * time.Millisecond)

	// Verify the path now uses the old configuration
	pathData, err = p.pathManager.APIPathsGet("undefined_stream")
	require.NoError(t, err)
	require.Equal(t, "undefined_stream", pathData.Name)
	require.Equal(t, "all", pathData.ConfName)
}
