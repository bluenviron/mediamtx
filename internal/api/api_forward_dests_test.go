package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/forward"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testForwardDestsPathManager struct {
	items map[uuid.UUID]*defs.APIForwardDest
}

func (*testForwardDestsPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{}, nil
}

func (*testForwardDestsPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	return &defs.APIPath{}, nil
}

func (m *testForwardDestsPathManager) APIForwardDestsList(path string) (*defs.APIForwardDestList, error) {
	if path != "my/nested/stream" {
		return nil, conf.ErrPathNotFound
	}

	items := make([]defs.APIForwardDest, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, *item)
	}

	return &defs.APIForwardDestList{Items: items}, nil
}

func (m *testForwardDestsPathManager) APIForwardDestsGet(path string, id uuid.UUID) (*defs.APIForwardDest, error) {
	if path != "my/nested/stream" {
		return nil, conf.ErrPathNotFound
	}

	item, ok := m.items[id]
	if !ok {
		return nil, forward.ErrDestNotFound
	}

	return item, nil
}

func (*testForwardDestsPathManager) APIStaticSourcesGet(string) (*defs.APIStaticSource, error) {
	return nil, conf.ErrPathNotFound
}

func TestForwardDests(t *testing.T) {
	rtmpID := uuid.New()
	moqID := uuid.New()
	whipID := uuid.New()
	pathManager := &testForwardDestsPathManager{
		items: map[uuid.UUID]*defs.APIForwardDest{
			rtmpID: {
				ID:            rtmpID,
				Pos:           1,
				Created:       time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
				Conf:          conf.ForwardDest{Dest: "rtmp://localhost/live/stream"},
				Type:          defs.APIForwardDestTypeRTMP,
				Protocol:      defs.APIForwardDestProtocolRTMP,
				State:         defs.APIForwardDestStateError,
				LastError:     "connection refused",
				OutboundBytes: 123,
			},
			moqID: {
				ID:      moqID,
				Pos:     2,
				Created: time.Date(2026, 6, 18, 9, 0, 30, 0, time.UTC),
				Conf: conf.ForwardDest{
					Dest:         "moqt://localhost/live/stream",
					MoQTransport: conf.MoQTransportWebTransport,
				},
				Type:          defs.APIForwardDestTypeMoQ,
				Protocol:      defs.APIForwardDestProtocolMoQ,
				State:         defs.APIForwardDestStateForwarding,
				OutboundBytes: 234,
			},
			whipID: {
				ID:            whipID,
				Pos:           3,
				Created:       time.Date(2026, 6, 18, 9, 1, 0, 0, time.UTC),
				Conf:          conf.ForwardDest{Dest: "whip://localhost/live/stream/whip", WHIPBearerToken: "mytoken"},
				Type:          defs.APIForwardDestTypeWebRTC,
				Protocol:      defs.APIForwardDestProtocolWHIP,
				State:         defs.APIForwardDestStateForwarding,
				OutboundBytes: 456,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		PathManager:  pathManager,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var list defs.APIForwardDestList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward-dests/list?path=my%2Fnested%2Fstream", nil, &list)
	require.Equal(t, 3, list.ItemCount)
	require.Equal(t, 1, list.PageCount)
	require.Len(t, list.Items, 3)

	require.ElementsMatch(t, []defs.APIForwardDest{
		{
			ID:      rtmpID,
			Pos:     1,
			Created: time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
			Conf: conf.ForwardDest{
				Dest:         "rtmp://localhost/live/stream",
				MoQTransport: conf.MoQTransportQUIC,
			},
			Type:          defs.APIForwardDestTypeRTMP,
			Protocol:      defs.APIForwardDestProtocolRTMP,
			State:         defs.APIForwardDestStateError,
			LastError:     "connection refused",
			OutboundBytes: 123,
		},
		{
			ID:      moqID,
			Pos:     2,
			Created: time.Date(2026, 6, 18, 9, 0, 30, 0, time.UTC),
			Conf: conf.ForwardDest{
				Dest:         "moqt://localhost/live/stream",
				MoQTransport: conf.MoQTransportWebTransport,
			},
			Type:          defs.APIForwardDestTypeMoQ,
			Protocol:      defs.APIForwardDestProtocolMoQ,
			State:         defs.APIForwardDestStateForwarding,
			OutboundBytes: 234,
		},
		{
			ID:      whipID,
			Pos:     3,
			Created: time.Date(2026, 6, 18, 9, 1, 0, 0, time.UTC),
			Conf: conf.ForwardDest{
				Dest:            "whip://localhost/live/stream/whip",
				WHIPBearerToken: "mytoken",
				MoQTransport:    conf.MoQTransportQUIC,
			},
			Type:          defs.APIForwardDestTypeWebRTC,
			Protocol:      defs.APIForwardDestProtocolWHIP,
			State:         defs.APIForwardDestStateForwarding,
			OutboundBytes: 456,
		},
	}, list.Items)

	var item defs.APIForwardDest
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward-dests/get?path=my%2Fnested%2Fstream&id="+moqID.String(), nil, &item)
	require.Equal(t, "moqt://localhost/live/stream", item.Conf.Dest)
	require.Equal(t, conf.MoQTransportWebTransport, item.Conf.MoQTransport)
	require.Equal(t, defs.APIForwardDestTypeMoQ, item.Type)
	require.Equal(t, defs.APIForwardDestProtocolMoQ, item.Protocol)
	require.Equal(t, defs.APIForwardDestStateForwarding, item.State)
	require.Equal(t, uint64(234), item.OutboundBytes)

	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/get?path=my%2Fnested%2Fstream&id="+moqID.String(), nil, &item)
}
