package forward_test

import (
	"net"
	"testing"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/forward"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestManager(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	done := make(chan struct{})

	go func() {
		conn, err2 := ln.Accept()
		require.NoError(t, err2)
		defer conn.Close()

		sc := &gortmplib.ServerConn{
			RW: conn,
		}
		err2 = sc.Initialize()
		require.NoError(t, err2)

		err2 = sc.Accept()
		require.NoError(t, err2)

		require.Equal(t, true, sc.Publish)
		require.Equal(t, "/app/stream", sc.URL.Path)

		close(done)
	}()

	m := &forward.Manager{
		PathName: "test",
		Forward: conf.Forward{
			{Dest: "rtmp://" + ln.Addr().String() + "/app/stream"},
		},
		Parent: test.NilLogger,
	}
	m.Initialize()

	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}}}

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	m.Start(strm)
	defer m.Stop()

	<-done
}

func TestManagerReloadConf(t *testing.T) {
	for _, ca := range []string{
		"idle",
		"running",
	} {
		t.Run(ca, func(t *testing.T) {
			m := &forward.Manager{
				PathName: "test",
				Forward: conf.Forward{
					{Dest: "rtmp://localhost:5788/app/stream"},
					{Dest: "rtsp://localhost:5789/stream"},
				},
				Parent: test.NilLogger,
			}
			m.Initialize()

			if ca == "running" {
				desc := &description.Session{Medias: []*description.Media{{
					Type:    description.MediaTypeVideo,
					Formats: []format.Format{test.FormatH264},
				}}}

				strm := &stream.Stream{
					OrigDesc:          desc,
					WriteQueueSize:    512,
					RTPMaxPayloadSize: 1450,
					Parent:            test.NilLogger,
				}
				require.NoError(t, strm.Initialize())
				defer strm.Close()

				m.Start(strm)
				defer m.Stop()
			}

			list1 := m.APIList()
			require.Equal(t, &defs.APIForwardDestList{
				Items: []defs.APIForwardDest{
					{
						ID:       list1.Items[0].ID,
						Pos:      1,
						Created:  list1.Items[0].Created,
						Conf:     conf.ForwardDest{Dest: "rtmp://localhost:5788/app/stream"},
						Protocol: "rtmp",
						State:    list1.Items[0].State,
					},
					{
						ID:       list1.Items[1].ID,
						Pos:      2,
						Created:  list1.Items[1].Created,
						Conf:     conf.ForwardDest{Dest: "rtsp://localhost:5789/stream"},
						Protocol: "rtsp",
						State:    list1.Items[1].State,
					},
				},
			}, list1)

			m.ReloadConf(conf.Forward{
				{Dest: "rtmp://localhost:5788/app/stream"}, // unchanged
				{Dest: "srt://localhost:5790?streamid=publish:test"},
				{Dest: "rtsp://localhost:5789/stream"},
			})

			list2 := m.APIList()
			require.Equal(t, &defs.APIForwardDestList{
				Items: []defs.APIForwardDest{
					{
						ID:        list1.Items[0].ID,
						Pos:       1,
						Created:   list1.Items[0].Created,
						Conf:      conf.ForwardDest{Dest: "rtmp://localhost:5788/app/stream"},
						Protocol:  "rtmp",
						State:     list2.Items[0].State,
						LastError: list2.Items[0].LastError,
					},
					{
						ID:        list2.Items[1].ID,
						Pos:       2,
						Created:   list2.Items[1].Created,
						Conf:      conf.ForwardDest{Dest: "srt://localhost:5790?streamid=publish:test"},
						Protocol:  "srt",
						State:     list2.Items[1].State,
						LastError: list2.Items[1].LastError,
					},
					{
						ID:       list2.Items[2].ID,
						Pos:      3,
						Created:  list2.Items[2].Created,
						Conf:     conf.ForwardDest{Dest: "rtsp://localhost:5789/stream"},
						Protocol: "rtsp",
						State:    list2.Items[2].State,
					},
				},
			}, list2)
		})
	}
}
