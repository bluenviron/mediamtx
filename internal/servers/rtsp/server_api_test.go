package rtsp_test

import (
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/rtsp"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestServerAPISessionsKickConcurrent(t *testing.T) {
	pathManager := &test.PathManager{
		FindPathConfImpl: func(defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
			return &defs.PathFindPathConfRes{Conf: &conf.Path{}}, nil
		},
	}

	s := &rtsp.Server{
		Address:        "127.0.0.1:8557",
		ReadTimeout:    conf.Duration(10 * time.Second),
		WriteTimeout:   conf.Duration(10 * time.Second),
		WriteQueueSize: 512,
		PathManager:    pathManager,
		Parent:         test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	for range 20 {
		u, err := base.ParseURL("rtsp://127.0.0.1:8557/teststream")
		require.NoError(t, err)
		source := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host}
		err = source.Start()
		require.NoError(t, err)

		_, err = source.Announce(u, &description.Session{Medias: []*description.Media{test.MediaH264}})
		require.NoError(t, err)

		list, err := s.APISessionsList()
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		id := list.Items[0].ID

		var wg sync.WaitGroup
		start := make(chan struct{})
		kickResults := make(chan error, 2)
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _ = s.APISessionsList()
				_, _ = s.APISessionsGet(id)
			}()
		}
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				kickResults <- s.APISessionsKick(id)
			}()
		}
		close(start)
		wg.Wait()
		close(kickResults)

		kicked, notFound := 0, 0
		for err := range kickResults {
			switch err {
			case nil:
				kicked++
			case rtsp.ErrSessionNotFound:
				notFound++
			default:
				require.NoError(t, err)
			}
		}
		require.Equal(t, 1, kicked)
		require.Equal(t, 1, notFound)

		list, err = s.APISessionsList()
		require.NoError(t, err)
		require.Empty(t, list.Items)
		require.ErrorIs(t, s.APISessionsKick(id), rtsp.ErrSessionNotFound)
		source.Close()
	}
}
