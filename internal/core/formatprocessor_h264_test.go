package core

import (
	"bytes"
	"net"
	"testing"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/base"
	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/conn"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/headers"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestFormatProcessorDynamicParams(t *testing.T) {
	checkTrack := func(t *testing.T, forma format.Format) {
		c := gortsplib.Client{}

		u, err := url.Parse("rtsp://127.0.0.1:8554/stream")
		require.NoError(t, err)

		err = c.Start(u.Scheme, u.Host)
		require.NoError(t, err)
		defer c.Close()

		medias, _, _, err := c.Describe(u)
		require.NoError(t, err)

		forma1 := medias[0].Formats[0]
		require.Equal(t, forma, forma1)
	}

	for _, ca := range []string{"h264", "h265"} {
		t.Run(ca, func(t *testing.T) {
			p, ok := newInstance("rtmpDisable: yes\n" +
				"hlsDisable: yes\n" +
				"webrtcDisable: yes\n" +
				"paths:\n" +
				"  all:\n")
			require.Equal(t, true, ok)
			defer p.Close()

			formah264 := &format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
			}

			formah265 := &format.H265{
				PayloadTyp: 96,
			}

			var forma format.Format
			if ca == "h264" {
				forma = formah264
			} else {
				forma = formah265
			}

			medi := &media.Media{
				Type:    media.TypeVideo,
				Formats: []format.Format{forma},
			}

			source := gortsplib.Client{}

			err := source.StartRecording(
				"rtsp://localhost:8554/stream",
				media.Medias{medi})
			require.NoError(t, err)
			defer source.Close()

			if ca == "h264" {
				enc := formah264.CreateEncoder()

				pkts, err := enc.Encode([][]byte{{7, 1, 2, 3}}, 0) // SPS
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				pkts, err = enc.Encode([][]byte{{8}}, 0) // PPS
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				checkTrack(t, &format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
					SPS:               []byte{7, 1, 2, 3},
					PPS:               []byte{8},
				})

				pkts, err = enc.Encode([][]byte{{7, 4, 5, 6}}, 0) // SPS
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				pkts, err = enc.Encode([][]byte{{8, 1}}, 0) // PPS
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				checkTrack(t, &format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
					SPS:               []byte{7, 4, 5, 6},
					PPS:               []byte{8, 1},
				})
			} else {
				enc := formah265.CreateEncoder()

				pkts, err := enc.Encode([][]byte{{byte(h265.NALUType_VPS_NUT) << 1, 1, 2, 3}}, 0)
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				pkts, err = enc.Encode([][]byte{{byte(h265.NALUType_SPS_NUT) << 1, 4, 5, 6}}, 0)
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				pkts, err = enc.Encode([][]byte{{byte(h265.NALUType_PPS_NUT) << 1, 7, 8, 9}}, 0)
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				checkTrack(t, &format.H265{
					PayloadTyp: 96,
					VPS:        []byte{byte(h265.NALUType_VPS_NUT) << 1, 1, 2, 3},
					SPS:        []byte{byte(h265.NALUType_SPS_NUT) << 1, 4, 5, 6},
					PPS:        []byte{byte(h265.NALUType_PPS_NUT) << 1, 7, 8, 9},
				})

				pkts, err = enc.Encode([][]byte{{byte(h265.NALUType_VPS_NUT) << 1, 10, 11, 12}}, 0)
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				pkts, err = enc.Encode([][]byte{{byte(h265.NALUType_SPS_NUT) << 1, 13, 14, 15}}, 0)
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				pkts, err = enc.Encode([][]byte{{byte(h265.NALUType_PPS_NUT) << 1, 16, 17, 18}}, 0)
				require.NoError(t, err)
				source.WritePacketRTP(medi, pkts[0])

				checkTrack(t, &format.H265{
					PayloadTyp: 96,
					VPS:        []byte{byte(h265.NALUType_VPS_NUT) << 1, 10, 11, 12},
					SPS:        []byte{byte(h265.NALUType_SPS_NUT) << 1, 13, 14, 15},
					PPS:        []byte{byte(h265.NALUType_PPS_NUT) << 1, 16, 17, 18},
				})
			}
		})
	}
}

func TestFormatProcessorOversizedPackets(t *testing.T) {
	for _, ca := range []string{"h264", "h265"} {
		t.Run(ca, func(t *testing.T) {
			p, ok := newInstance("rtmpDisable: yes\n" +
				"hlsDisable: yes\n" +
				"webrtcDisable: yes\n" +
				"paths:\n" +
				"  all:\n")
			require.Equal(t, true, ok)
			defer p.Close()

			nconn, err := net.Dial("tcp", "localhost:8554")
			require.NoError(t, err)
			defer nconn.Close()
			conn := conn.NewConn(nconn)

			var forma format.Format
			if ca == "h264" {
				forma = &format.H264{
					PayloadTyp:        96,
					SPS:               []byte{0x01, 0x02, 0x03, 0x04},
					PPS:               []byte{0x01, 0x02, 0x03, 0x04},
					PacketizationMode: 1,
				}
			} else {
				forma = &format.H265{
					PayloadTyp: 96,
					VPS:        []byte{byte(h265.NALUType_VPS_NUT) << 1, 10, 11, 12},
					SPS:        []byte{byte(h265.NALUType_SPS_NUT) << 1, 13, 14, 15},
					PPS:        []byte{byte(h265.NALUType_PPS_NUT) << 1, 16, 17, 18},
				}
			}

			medi := &media.Media{
				Type:    media.TypeVideo,
				Formats: []format.Format{forma},
			}
			medias := media.Medias{medi}
			medias.SetControls()

			res, err := writeReqReadRes(conn, base.Request{
				Method: base.Announce,
				URL:    mustParseURL("rtsp://localhost:8554/stream"),
				Header: base.Header{
					"CSeq":         base.HeaderValue{"1"},
					"Content-Type": base.HeaderValue{"application/sdp"},
				},
				Body: mustMarshalSDP(medias.Marshal(false)),
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)

			var sx headers.Session

			inTH := &headers.Transport{
				Delivery: func() *headers.TransportDelivery {
					v := headers.TransportDeliveryUnicast
					return &v
				}(),
				Mode: func() *headers.TransportMode {
					v := headers.TransportModeRecord
					return &v
				}(),
				Protocol:       headers.TransportProtocolTCP,
				InterleavedIDs: &[2]int{0, 1},
			}

			res, err = writeReqReadRes(conn, base.Request{
				Method: base.Setup,
				URL:    mustParseURL("rtsp://localhost:8554/stream/" + medias[0].Control),
				Header: base.Header{
					"CSeq":      base.HeaderValue{"2"},
					"Transport": inTH.Marshal(),
				},
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)

			err = sx.Unmarshal(res.Header["Session"])
			require.NoError(t, err)

			res, err = writeReqReadRes(conn, base.Request{
				Method: base.Record,
				URL:    mustParseURL("rtsp://localhost:8554/stream"),
				Header: base.Header{
					"CSeq":    base.HeaderValue{"3"},
					"Session": base.HeaderValue{sx.Session},
				},
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://127.0.0.1:8554/stream")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(medias, baseURL)
			require.NoError(t, err)

			packetRecv := make(chan struct{})
			i := 0

			var expected []*rtp.Packet

			if ca == "h264" {
				expected = []*rtp.Packet{
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
							CSRC:           []uint32{},
						},
						Payload: []byte{0x01, 0x02, 0x03, 0x04},
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         false,
							PayloadType:    96,
							SequenceNumber: 124,
							Timestamp:      45343,
							SSRC:           563423,
							CSRC:           []uint32{},
						},
						Payload: append(
							append([]byte{0x1c, 0x80}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 364)...),
							[]byte{0x01, 0x02}...,
						),
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 125,
							Timestamp:      45343,
							SSRC:           563423,
							CSRC:           []uint32{},
						},
						Payload: append(
							[]byte{0x1c, 0x40, 0x03, 0x04},
							bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 136)...,
						),
					},
				}
			} else {
				expected = []*rtp.Packet{
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
							CSRC:           []uint32{},
						},
						Payload: []byte{0x01, 0x02, 0x03, 0x04},
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         false,
							PayloadType:    96,
							SequenceNumber: 124,
							Timestamp:      45343,
							SSRC:           563423,
							CSRC:           []uint32{},
						},
						Payload: append(
							append([]byte{0x63, 0x02, 0x80, 0x03, 0x04}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 363)...),
							[]byte{0x01, 0x02, 0x03}...,
						),
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 125,
							Timestamp:      45343,
							SSRC:           563423,
							CSRC:           []uint32{},
						},
						Payload: append(
							[]byte{0x63, 0x02, 0x40, 0x04},
							bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 135)...,
						),
					},
				}
			}

			c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
				require.Equal(t, expected[i], pkt)
				i++
				if i >= len(expected) {
					close(packetRecv)
				}
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			var tosend []*rtp.Packet
			if ca == "h264" {
				tosend = []*rtp.Packet{
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
							Padding:        true,
						},
						Payload: []byte{0x01, 0x02, 0x03, 0x04},
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         false,
							PayloadType:    96,
							SequenceNumber: 124,
							Timestamp:      45343,
							SSRC:           563423,
							Padding:        true,
						},
						Payload: append([]byte{0x1c, 0b10000000}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4)...),
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 125,
							Timestamp:      45343,
							SSRC:           563423,
							Padding:        true,
						},
						Payload: []byte{0x1c, 0b01000000, 0x01, 0x02, 0x03, 0x04},
					},
				}
			} else {
				tosend = []*rtp.Packet{
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
							Padding:        true,
						},
						Payload: []byte{0x01, 0x02, 0x03, 0x04},
					},
					{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 124,
							Timestamp:      45343,
							SSRC:           563423,
							Padding:        true,
						},
						Payload: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4),
					},
				}
			}

			for _, pkt := range tosend {
				byts, _ := pkt.Marshal()
				err = conn.WriteInterleavedFrame(&base.InterleavedFrame{
					Channel: 0,
					Payload: byts,
				}, make([]byte, 2048))
				require.NoError(t, err)
			}

			<-packetRecv
		})
	}
}
