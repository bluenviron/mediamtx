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

		err2 = sc.AcceptConn()
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
					{Dest: "moqt://localhost:5791/teststream", MoQTransport: conf.MoQTransportWebTransport},
					{Dest: "whip://localhost:5790/teststream/whip", WHIPBearerToken: "mytoken"},
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
						Type:     defs.APIForwardDestTypeRTMP,
						Protocol: "rtmp",
						State:    list1.Items[0].State,
					},
					{
						ID:       list1.Items[1].ID,
						Pos:      2,
						Created:  list1.Items[1].Created,
						Conf:     conf.ForwardDest{Dest: "rtsp://localhost:5789/stream"},
						Type:     defs.APIForwardDestTypeRTSP,
						Protocol: "rtsp",
						State:    list1.Items[1].State,
					},
					{
						ID:      list1.Items[2].ID,
						Pos:     3,
						Created: list1.Items[2].Created,
						Conf: conf.ForwardDest{
							Dest:         "moqt://localhost:5791/teststream",
							MoQTransport: conf.MoQTransportWebTransport,
						},
						Type:      defs.APIForwardDestTypeMoQ,
						Protocol:  "moq",
						State:     list1.Items[2].State,
						LastError: list1.Items[2].LastError,
					},
					{
						ID:      list1.Items[3].ID,
						Pos:     4,
						Created: list1.Items[3].Created,
						Conf: conf.ForwardDest{
							Dest:            "whip://localhost:5790/teststream/whip",
							WHIPBearerToken: "mytoken",
						},
						Type:      defs.APIForwardDestTypeWebRTC,
						Protocol:  "whip",
						State:     list1.Items[3].State,
						LastError: list1.Items[3].LastError,
					},
				},
			}, list1)

			m.ReloadConf(conf.Forward{
				{Dest: "rtmp://localhost:5788/app/stream"},
				{Dest: "rtsp://localhost:5789/stream", DestFingerprint: "fingerprint"},
				{Dest: "moqt://localhost:5791/teststream", MoQTransport: conf.MoQTransportQUIC},
				{Dest: "whip://localhost:5790/teststream/whip", WHIPBearerToken: "othertoken"},
			})

			list2 := m.APIList()
			require.Equal(t, &defs.APIForwardDestList{
				Items: []defs.APIForwardDest{
					{
						ID:        list1.Items[0].ID,
						Pos:       1,
						Created:   list1.Items[0].Created,
						Conf:      conf.ForwardDest{Dest: "rtmp://localhost:5788/app/stream"},
						Type:      defs.APIForwardDestTypeRTMP,
						Protocol:  "rtmp",
						State:     list2.Items[0].State,
						LastError: list2.Items[0].LastError,
					},
					{
						ID:      list2.Items[1].ID,
						Pos:     2,
						Created: list2.Items[1].Created,
						Conf: conf.ForwardDest{
							Dest:            "rtsp://localhost:5789/stream",
							DestFingerprint: "fingerprint",
						},
						Type:      defs.APIForwardDestTypeRTSP,
						Protocol:  "rtsp",
						State:     list2.Items[1].State,
						LastError: list2.Items[1].LastError,
					},
					{
						ID:      list2.Items[2].ID,
						Pos:     3,
						Created: list2.Items[2].Created,
						Conf: conf.ForwardDest{
							Dest:         "moqt://localhost:5791/teststream",
							MoQTransport: conf.MoQTransportQUIC,
						},
						Type:      defs.APIForwardDestTypeMoQ,
						Protocol:  "moq",
						State:     list2.Items[2].State,
						LastError: list2.Items[2].LastError,
					},
					{
						ID:      list2.Items[3].ID,
						Pos:     4,
						Created: list2.Items[3].Created,
						Conf: conf.ForwardDest{
							Dest:            "whip://localhost:5790/teststream/whip",
							WHIPBearerToken: "othertoken",
						},
						Type:      defs.APIForwardDestTypeWebRTC,
						Protocol:  "whip",
						State:     list2.Items[3].State,
						LastError: list2.Items[3].LastError,
					},
				},
			}, list2)

			require.Equal(t, list1.Items[0].ID, list2.Items[0].ID)
			require.NotEqual(t, list1.Items[1].ID, list2.Items[1].ID)
			require.NotEqual(t, list1.Items[2].ID, list2.Items[2].ID)
			require.NotEqual(t, list1.Items[3].ID, list2.Items[3].ID)
		})
	}
}

func TestManagerReloadConfDoesNotRestartOtherDestinations(t *testing.T) {
	for _, ca := range []string{
		"idle",
		"running",
	} {
		t.Run(ca, func(t *testing.T) {
			dest := func(n string) conf.ForwardDest {
				return conf.ForwardDest{Dest: "rtmp://localhost:5788/app/" + n}
			}

			m := &forward.Manager{
				PathName: "test",
				Forward:  conf.Forward{dest("a"), dest("b"), dest("c"), dest("d")},
				Parent:   test.NilLogger,
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

			// ids returns the ID and position of each destination, by destination name.
			ids := func() map[string][2]any {
				ret := make(map[string][2]any)
				for _, item := range m.APIList().Items {
					ret[item.Conf.Dest[len("rtmp://localhost:5788/app/"):]] = [2]any{item.ID, item.Pos}
				}
				return ret
			}

			initial := ids()
			require.Len(t, initial, 4)

			// remove a destination in the middle: the others are neither restarted nor affected.
			m.ReloadConf(conf.Forward{dest("a"), dest("c"), dest("d")})
			after := ids()
			require.Len(t, after, 3)
			require.Equal(t, initial["a"][0], after["a"][0])
			require.Equal(t, initial["c"][0], after["c"][0])
			require.Equal(t, initial["d"][0], after["d"][0])
			require.Equal(t, 1, after["a"][1])
			require.Equal(t, 2, after["c"][1])
			require.Equal(t, 3, after["d"][1])

			// insert a destination in the middle: only the new one is created.
			m.ReloadConf(conf.Forward{dest("a"), dest("x"), dest("c"), dest("d")})
			after = ids()
			require.Len(t, after, 4)
			require.Equal(t, initial["a"][0], after["a"][0])
			require.Equal(t, initial["c"][0], after["c"][0])
			require.Equal(t, initial["d"][0], after["d"][0])
			require.NotEqual(t, initial["b"][0], after["x"][0])
			require.Equal(t, 2, after["x"][1])
			require.Equal(t, 4, after["d"][1])

			// reorder destinations: nothing is restarted.
			m.ReloadConf(conf.Forward{dest("d"), dest("c"), dest("x"), dest("a")})
			after = ids()
			require.Equal(t, initial["a"][0], after["a"][0])
			require.Equal(t, initial["c"][0], after["c"][0])
			require.Equal(t, initial["d"][0], after["d"][0])
			require.Equal(t, 1, after["d"][1])
			require.Equal(t, 4, after["a"][1])

			// a changed destination is restarted, even if it keeps its position.
			changed := conf.ForwardDest{Dest: dest("c").Dest, DestFingerprint: "fingerprint"}
			m.ReloadConf(conf.Forward{dest("d"), changed, dest("x"), dest("a")})
			after = ids()
			require.Equal(t, initial["d"][0], after["d"][0])
			require.NotEqual(t, initial["c"][0], after["c"][0])
			require.Equal(t, initial["a"][0], after["a"][0])
		})
	}
}

func TestManagerReloadConfDuplicateDestinations(t *testing.T) {
	dest := conf.ForwardDest{Dest: "rtmp://localhost:5788/app/stream"}

	m := &forward.Manager{
		PathName: "test",
		Forward:  conf.Forward{dest, dest},
		Parent:   test.NilLogger,
	}
	m.Initialize()

	list1 := m.APIList()
	require.Len(t, list1.Items, 2)
	require.NotEqual(t, list1.Items[0].ID, list1.Items[1].ID)

	// removing one of two identical destinations keeps exactly one of them.
	m.ReloadConf(conf.Forward{dest})
	list2 := m.APIList()
	require.Len(t, list2.Items, 1)
	require.Equal(t, list1.Items[0].ID, list2.Items[0].ID)

	// adding one back creates a new one and keeps the existing one.
	m.ReloadConf(conf.Forward{dest, dest})
	list3 := m.APIList()
	require.Len(t, list3.Items, 2)
	require.Equal(t, list1.Items[0].ID, list3.Items[0].ID)
	require.NotEqual(t, list1.Items[1].ID, list3.Items[1].ID)
}
