package api //nolint:revive

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/srt"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testSRTServer struct {
	conns map[uuid.UUID]*defs.APISRTConn
}

func (s *testSRTServer) APIConnsList() (*defs.APISRTConnList, error) {
	items := make([]defs.APISRTConn, 0, len(s.conns))
	for _, conn := range s.conns {
		items = append(items, *conn)
	}
	return &defs.APISRTConnList{Items: items}, nil
}

func (s *testSRTServer) APIConnsGet(id uuid.UUID) (*defs.APISRTConn, error) {
	conn, ok := s.conns[id]
	if !ok {
		return nil, srt.ErrConnNotFound
	}
	return conn, nil
}

func (s *testSRTServer) APIConnsKick(id uuid.UUID) error {
	_, ok := s.conns[id]
	if !ok {
		return srt.ErrConnNotFound
	}
	return nil
}

func TestSRTConnsList(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	now := time.Now()

	srtServer := &testSRTServer{
		conns: map[uuid.UUID]*defs.APISRTConn{
			id1: {
				ID:                    id1,
				Created:               now,
				RemoteAddr:            "192.168.1.1:5000",
				State:                 defs.APISRTConnStatePublish,
				Path:                  "stream1",
				Query:                 "token=abc",
				PacketsSent:           1000,
				PacketsReceived:       2000,
				PacketsSentUnique:     950,
				PacketsReceivedUnique: 1950,
				BytesReceived:         100000,
				BytesSent:             200000,
				MsRTT:                 10.5,
				MbpsSendRate:          5.2,
				MbpsReceiveRate:       4.8,
			},
			id2: {
				ID:                    id2,
				Created:               now.Add(time.Minute),
				RemoteAddr:            "192.168.1.2:5001",
				State:                 defs.APISRTConnStateRead,
				Path:                  "stream2",
				Query:                 "",
				PacketsSent:           500,
				PacketsReceived:       1500,
				PacketsSentUnique:     480,
				PacketsReceivedUnique: 1470,
				BytesReceived:         50000,
				BytesSent:             150000,
				MsRTT:                 15.2,
				MbpsSendRate:          3.5,
				MbpsReceiveRate:       3.2,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		SRTServer:    srtServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APISRTConnList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/srtconns/list", nil, &out)

	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Len(t, out.Items, 2)
}

func TestSRTConnsGet(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	srtServer := &testSRTServer{
		conns: map[uuid.UUID]*defs.APISRTConn{
			id: {
				ID:                            id,
				Created:                       now,
				RemoteAddr:                    "192.168.1.100:5000",
				State:                         defs.APISRTConnStatePublish,
				Path:                          "mystream",
				Query:                         "key=value",
				PacketsSent:                   10000,
				PacketsReceived:               20000,
				PacketsSentUnique:             9900,
				PacketsReceivedUnique:         19800,
				PacketsSendLoss:               50,
				PacketsReceivedLoss:           100,
				PacketsRetrans:                60,
				PacketsReceivedRetrans:        80,
				PacketsSentACK:                500,
				PacketsReceivedACK:            600,
				PacketsSentNAK:                10,
				PacketsReceivedNAK:            15,
				PacketsSentKM:                 2,
				PacketsReceivedKM:             2,
				UsSndDuration:                 1000000,
				PacketsReceivedBelated:        5,
				PacketsSendDrop:               3,
				PacketsReceivedDrop:           4,
				PacketsReceivedUndecrypt:      0,
				BytesReceived:                 999999,
				BytesSent:                     888888,
				BytesSentUnique:               880000,
				BytesReceivedUnique:           990000,
				BytesReceivedLoss:             5000,
				BytesRetrans:                  3000,
				BytesReceivedRetrans:          4000,
				BytesReceivedBelated:          200,
				BytesSendDrop:                 150,
				BytesReceivedDrop:             180,
				BytesReceivedUndecrypt:        0,
				UsPacketsSendPeriod:           1000.5,
				PacketsFlowWindow:             8192,
				PacketsFlightSize:             256,
				MsRTT:                         25.5,
				MbpsSendRate:                  10.5,
				MbpsReceiveRate:               9.8,
				MbpsLinkCapacity:              100.0,
				BytesAvailSendBuf:             65536,
				BytesAvailReceiveBuf:          131072,
				MbpsMaxBW:                     50.0,
				ByteMSS:                       1500,
				PacketsSendBuf:                128,
				BytesSendBuf:                  192000,
				MsSendBuf:                     1000,
				MsSendTsbPdDelay:              120,
				PacketsReceiveBuf:             256,
				BytesReceiveBuf:               384000,
				MsReceiveBuf:                  2000,
				MsReceiveTsbPdDelay:           120,
				PacketsReorderTolerance:       10,
				PacketsReceivedAvgBelatedTime: 50,
				PacketsSendLossRate:           0.5,
				PacketsReceivedLossRate:       0.6,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		SRTServer:    srtServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APISRTConn
	httpRequest(t, hc, http.MethodGet, fmt.Sprintf("http://localhost:9997/v3/srtconns/get/%s", id), nil, &out)

	require.Equal(t, id, out.ID)
	require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
	require.Equal(t, defs.APISRTConnStatePublish, out.State)
	require.Equal(t, "mystream", out.Path)
	require.Equal(t, uint64(999999), out.BytesReceived)
	require.Equal(t, uint64(888888), out.BytesSent)
	require.Equal(t, 25.5, out.MsRTT)
	require.Equal(t, 10.5, out.MbpsSendRate)
	require.Equal(t, 9.8, out.MbpsReceiveRate)
}

func TestSRTConnsKick(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	srtServer := &testSRTServer{
		conns: map[uuid.UUID]*defs.APISRTConn{
			id: {
				ID:                    id,
				Created:               now,
				RemoteAddr:            "192.168.1.100:5000",
				State:                 defs.APISRTConnStatePublish,
				Path:                  "mystream",
				Query:                 "",
				PacketsSent:           1000,
				PacketsReceived:       2000,
				PacketsSentUnique:     950,
				PacketsReceivedUnique: 1950,
				BytesReceived:         100000,
				BytesSent:             200000,
				MsRTT:                 10.5,
				MbpsSendRate:          5.2,
				MbpsReceiveRate:       4.8,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		SRTServer:    srtServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, fmt.Sprintf("http://localhost:9997/v3/srtconns/kick/%s", id), nil, nil)
}
