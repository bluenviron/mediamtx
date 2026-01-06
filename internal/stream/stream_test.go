package stream

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type nilLogger struct{}

func (nilLogger) Log(logger.Level, string, ...any) {
}

func TestStream(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
	}}

	strm := &Stream{
		Desc:              desc,
		UseRTPPackets:     false,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	r := &Reader{}

	recv := make(chan struct{})

	r.OnData(desc.Medias[0], desc.Medias[0].Formats[0], func(_ *unit.Unit) error {
		close(recv)
		return nil
	})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 30000 * 2,
		Payload: unit.PayloadH264{
			{5, 2}, // IDR
		},
	})

	<-recv

	require.Equal(t, uint64(14), strm.BytesReceived())
	require.Equal(t, uint64(14), strm.BytesSent())
}

func TestStreamSkipBytesSent(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
	}}

	strm := &Stream{
		Desc:              desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	r := &Reader{
		SkipBytesSent: true,
	}

	recv := make(chan struct{})

	r.OnData(desc.Medias[0], desc.Medias[0].Formats[0], func(_ *unit.Unit) error {
		close(recv)
		return nil
	})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 30000 * 2,
		Payload: unit.PayloadH264{
			{5, 2}, // IDR
		},
	})

	<-recv

	require.Equal(t, uint64(14), strm.BytesReceived())
	require.Equal(t, uint64(0), strm.BytesSent())
}

func TestStreamResizeOversizedRTPPackets(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type: description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{
				SPS: []byte{ // 1920x1080 baseline
					0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
					0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
					0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
				},
				PPS: []byte{0x08, 0x06, 0x07, 0x08},
			}},
		},
	}}

	strm := &Stream{
		Desc:              desc,
		UseRTPPackets:     true,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 400,
		Parent:            &nilLogger{},
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	r := &Reader{}

	recv := make(chan *unit.Unit)
	n := 0

	r.OnData(desc.Medias[0], desc.Medias[0].Formats[0], func(u *unit.Unit) error {
		switch n {
		case 0:
		case 1:
			recv <- u
		default:
			t.Error("should not happen")
		}
		n++
		return nil
	})

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 90000,
		RTPPackets: []*rtp.Packet{
			{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    96,
					SequenceNumber: 122,
					Timestamp:      45343,
					SSRC:           563423,
				},
				Payload: []byte{1, 2, 3, 4},
			},
		},
	})

	oversizedPayload := make([]byte, 1000)
	for i := range oversizedPayload {
		oversizedPayload[i] = byte(i % 256)
	}

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 90000,
		RTPPackets: []*rtp.Packet{
			{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    96,
					SequenceNumber: 123,
					Timestamp:      45343,
					SSRC:           563423,
				},
				Payload: oversizedPayload,
			},
		},
	})

	received := <-recv

	require.Equal(t, 3, len(received.RTPPackets))

	for i, pkt := range received.RTPPackets {
		require.Equal(t, 123+uint16(i), pkt.SequenceNumber)
	}

	totalPayloadSize := 0
	for _, pkt := range received.RTPPackets {
		require.LessOrEqual(t, len(pkt.Payload), 400)
		totalPayloadSize += len(pkt.Payload)
	}

	require.Equal(t, 1005, totalPayloadSize)
}

func TestStreamUpdateFormatParams(t *testing.T) {
	for _, ca := range []string{"h264", "h265", "mpeg4video"} {
		t.Run(ca, func(t *testing.T) {
			var desc *description.Session
			var media *description.Media
			var forma format.Format
			var u *unit.Unit

			switch ca {
			case "h264":
				sps := []byte{
					0x67, 0x64, 0x00, 0x20, 0xac, 0xd9, 0x40, 0x78,
					0x02, 0x27, 0xe5, 0x9a, 0x80, 0x80, 0x80, 0xa0,
				}
				pps := []byte{0x08, 0x07, 0x08, 0x09}

				formatH264 := &format.H264{
					SPS: []byte{0x67, 0x42, 0xc0, 0x28},
					PPS: []byte{0x08, 0x06},
				}

				desc = &description.Session{Medias: []*description.Media{{
					Type:    description.MediaTypeVideo,
					Formats: []format.Format{formatH264},
				}}}
				media = desc.Medias[0]
				forma = formatH264

				u = &unit.Unit{
					PTS: 90000,
					Payload: unit.PayloadH264{
						sps,    // New SPS
						pps,    // New PPS
						{5, 1}, // IDR
					},
				}

			case "h265":
				vps := []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xfe}
				sps := []byte{0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x04}
				pps := []byte{0x44, 0x01, 0xc1, 0x73, 0xd1, 0x8a}

				formatH265 := &format.H265{
					VPS: []byte{0x40, 0x01, 0x0c},
					SPS: []byte{0x42, 0x01, 0x01},
					PPS: []byte{0x44, 0x01, 0xc1},
				}

				desc = &description.Session{Medias: []*description.Media{{
					Type:    description.MediaTypeVideo,
					Formats: []format.Format{formatH265},
				}}}
				media = desc.Medias[0]
				forma = formatH265

				u = &unit.Unit{
					PTS: 90000,
					Payload: unit.PayloadH265{
						vps,                      // New VPS
						sps,                      // New SPS
						pps,                      // New PPS
						{0x26, 0x01, 0x01, 0x02}, // IDR
					},
				}

			case "mpeg4video":
				config := []byte{
					0x00, 0x00, 0x01, 0xb0, // Visual Object Sequence Start
					0x02, 0x00, 0x00, 0x01, 0xb5, 0x8a,
					0x14, 0x00, 0x00, 0x01, 0x00,
				}

				formatMPEG4Video := &format.MPEG4Video{
					Config: []byte{0x00, 0x00, 0x01, 0xb0, 0x01},
				}

				desc = &description.Session{Medias: []*description.Media{{
					Type:    description.MediaTypeVideo,
					Formats: []format.Format{formatMPEG4Video},
				}}}
				media = desc.Medias[0]
				forma = formatMPEG4Video

				frame := make([]byte, 0, len(config)+20)
				frame = append(frame, config...)
				frame = append(frame, []byte{0x00, 0x00, 0x01, 0xb3}...) // Group of VOP
				frame = append(frame, []byte{0x01, 0x02, 0x03, 0x04}...)

				u = &unit.Unit{
					PTS:     90000,
					Payload: unit.PayloadMPEG4Video(frame),
				}
			}

			strm := &Stream{
				Desc:              desc,
				UseRTPPackets:     false,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			r := &Reader{}
			recv := make(chan struct{})

			r.OnData(media, forma, func(_ *unit.Unit) error {
				close(recv)
				return nil
			})

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			strm.WriteUnit(media, forma, u)
			<-recv

			// Verify that format parameters were updated
			switch ca {
			case "h264":
				formatH264 := forma.(*format.H264)
				require.Equal(t, []byte{
					0x67, 0x64, 0x00, 0x20, 0xac, 0xd9, 0x40, 0x78,
					0x02, 0x27, 0xe5, 0x9a, 0x80, 0x80, 0x80, 0xa0,
				}, formatH264.SPS)
				require.Equal(t, []byte{0x08, 0x07, 0x08, 0x09}, formatH264.PPS)

			case "h265":
				formatH265 := forma.(*format.H265)
				require.Equal(t, []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xfe}, formatH265.VPS)
				require.Equal(t, []byte{0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x04}, formatH265.SPS)
				require.Equal(t, []byte{0x44, 0x01, 0xc1, 0x73, 0xd1, 0x8a}, formatH265.PPS)

			case "mpeg4video":
				formatMPEG4Video := forma.(*format.MPEG4Video)
				require.Equal(t, []byte{
					0x00, 0x00, 0x01, 0xb0,
					0x02, 0x00, 0x00, 0x01, 0xb5, 0x8a,
					0x14, 0x00, 0x00, 0x01, 0x00,
				}, formatMPEG4Video.Config)
			}
		})
	}
}

func TestStreamDecode(t *testing.T) {
	for _, ca := range []struct {
		name    string
		format  format.Format
		encoded []*rtp.Packet
		decoded unit.Payload
	}{
		{
			name:   "av1",
			format: &format.AV1{},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						0b00011000, // Z=0, N=0, Y=1, W=1 (1 OBU, 2 bytes size)
						0x02,       // Size = 2
						0x01, 0x02, // OBU data
					},
				},
			},
			decoded: unit.PayloadAV1{
				{0x02, 0x01, 0x02}, // Size byte included with OBU data
			},
		},
		{
			name:   "vp9",
			format: &format.VP9{},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						0x9c, // I=1, P=0, L=0, F=1, B=1, E=1, V=0, Z=0
						0x01,
						0x02, 0x03, 0x04, // VP9 frame data
					},
				},
			},
			decoded: unit.PayloadVP9{0x02, 0x03, 0x04},
		},
		{
			name:   "vp8",
			format: &format.VP8{},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						0x10, // X=0, R=0, N=0, S=1, PartID=0
						0x01, 0x02, 0x03, 0x04,
					},
				},
			},
			decoded: unit.PayloadVP8{0x01, 0x02, 0x03, 0x04},
		},
		{
			name: "h265",
			format: &format.H265{
				VPS: []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xfe},
				SPS: []byte{0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x04},
				PPS: []byte{0x44, 0x01, 0xc1, 0x73, 0xd1, 0x8a},
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						0x26, 0x01, 0x01, 0x02, 0x03, // IDR
					},
				},
			},
			decoded: unit.PayloadH265{
				{0x40, 0x01, 0x0c, 0x01, 0xff, 0xfe},             // VPS
				{0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x04}, // SPS
				{0x44, 0x01, 0xc1, 0x73, 0xd1, 0x8a},             // PPS
				{0x26, 0x01, 0x01, 0x02, 0x03},                   // IDR
			},
		},
		{
			name: "h264",
			format: &format.H264{
				SPS: []byte{
					0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
					0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
					0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
				},
				PPS: []byte{0x08, 0x06, 0x07, 0x08},
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						0x05, 0x01, 0x02, 0x03, 0x04, // IDR
					},
				},
			},
			decoded: unit.PayloadH264{
				{
					0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84,
					0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
				}, // SPS
				{0x08, 0x06, 0x07, 0x08},       // PPS
				{0x05, 0x01, 0x02, 0x03, 0x04}, // IDR
			},
		},
		{
			name: "mpeg4video",
			format: &format.MPEG4Video{
				Config: []byte{0x00, 0x00, 0x01, 0xb0, 0x01},
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{0x00, 0x01, 0x02, 0x03, 0x04},
				},
			},
			decoded: unit.PayloadMPEG4Video{0x00, 0x01, 0x02, 0x03, 0x04},
		},
		{
			name:   "mpeg1video",
			format: &format.MPEG1Video{},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true, // Marker indicates complete frame
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						// MPEG-1 Video RTP header (4 bytes)
						0x00, // MBZ=0, T=0 (MPEG-1)
						0x00, // TR (temporal reference) - low 8 bits
						0x18, // AN=0, N=0, S=0 (no sequence header), B=1, E=1 (complete slice), FBV=0, BFC=0, FFV=0, FFC=0
						0x00, // FFC (continued)
						// MPEG-1 Video data (slice or frame data)
						0x00, 0x00, 0x01, 0x01, // Slice start code
						0x01, 0x02, 0x03, 0x04, // Slice data
					},
				},
			},
			decoded: unit.PayloadMPEG1Video{
				// Only the video data after the 4-byte RTP header
				0x00, 0x00, 0x01, 0x01,
				0x01, 0x02, 0x03, 0x04,
			},
		},
		{
			name: "mpeg4audio",
			format: &format.MPEG4Audio{
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						// AU-headers-length: 16 bits (2 bytes) = 16 bits of headers
						0x00, 0x10,
						// AU-header: 13 bits size + 3 bits index
						// size=4 (13 bits): 0000000000100
						// index=0 (3 bits): 000
						// Combined: 0000000000100000 = 0x0020
						0x00, 0x20,
						// AU data
						0x01, 0x02, 0x03, 0x04,
					},
				},
			},
			decoded: unit.PayloadMPEG4Audio{
				{0x01, 0x02, 0x03, 0x04},
			},
		},
		{
			name: "opus",
			format: &format.Opus{
				ChannelCount: 2,
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         false,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{0x01, 0x02, 0x03, 0x04},
				},
			},
			decoded: unit.PayloadOpus{
				{0x01, 0x02, 0x03, 0x04},
			},
		},
		{
			name: "g711",
			format: &format.G711{
				MULaw:        true,
				SampleRate:   8000,
				ChannelCount: 1,
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         false,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{0x01, 0x02, 0x03, 0x04},
				},
			},
			decoded: unit.PayloadG711{0x01, 0x02, 0x03, 0x04},
		},
		{
			name: "lpcm",
			format: &format.LPCM{
				BitDepth:     16,
				SampleRate:   48000,
				ChannelCount: 2,
			},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         false,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{0x01, 0x02, 0x03, 0x04},
				},
			},
			decoded: unit.PayloadLPCM{0x01, 0x02, 0x03, 0x04},
		},
		{
			name:   "klv",
			format: &format.KLV{},
			encoded: []*rtp.Packet{
				{
					Header: rtp.Header{
						Version:        2,
						Marker:         true, // Marker bit indicates complete KLV unit
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
					},
					Payload: []byte{
						// KLV Universal Label Key (16 bytes) - starts with 0x060e2b34
						0x06, 0x0e, 0x2b, 0x34, 0x01, 0x01, 0x01, 0x01,
						0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
						// Length (1 byte, short form: 4 bytes of data)
						0x04,
						// Value (4 bytes)
						0x01, 0x02, 0x03, 0x04,
					},
				},
			},
			decoded: unit.PayloadKLV{
				// Complete KLV unit: Universal Label Key + Length + Value
				0x06, 0x0e, 0x2b, 0x34, 0x01, 0x01, 0x01, 0x01,
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x04,
				0x01, 0x02, 0x03, 0x04,
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				{
					Formats: []format.Format{ca.format},
				},
			}}

			strm := &Stream{
				Desc:              desc,
				UseRTPPackets:     true,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            &nilLogger{},
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			r := &Reader{}
			recv := make(chan *unit.Unit)

			r.OnData(desc.Medias[0], ca.format, func(u *unit.Unit) error {
				recv <- u
				close(recv)
				return nil
			})

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			strm.WriteUnit(desc.Medias[0], ca.format, &unit.Unit{
				RTPPackets: ca.encoded,
			})

			received := <-recv
			if ca.decoded != nil {
				require.Equal(t, ca.decoded, received.Payload)
			} else {
				// For formats that construct complete frames (MJPEG, MPEG-1 Audio, AC-3),
				// just verify that we got data
				require.NotNil(t, received.Payload)
			}
		})
	}
}
