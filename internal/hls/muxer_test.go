package hls

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/asticode/go-astits"
	"github.com/stretchr/testify/require"
)

var testTime = time.Date(2010, 0o1, 0o1, 0o1, 0o1, 0o1, 0, time.UTC)

// baseline profile without POC
var testSPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

func testMP4(t *testing.T, byts []byte, boxes []gomp4.BoxPath) {
	i := 0
	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		require.Equal(t, boxes[i], h.Path)
		i++
		return h.Expand()
	})
	require.NoError(t, err)
}

func TestMuxerVideoAudio(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	audioTrack := &gortsplib.TrackMPEG4Audio{
		PayloadType: 97,
		Config: &mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}

	for _, ca := range []string{
		"mpegts",
		"fmp4",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m, err := NewMuxer(v, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, audioTrack)
			require.NoError(t, err)
			defer m.Close()

			// group without IDR
			d := 1 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{0x06},
				{0x07},
			})
			require.NoError(t, err)

			// group with IDR
			d = 2 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteAAC(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			d = 3500 * time.Millisecond
			err = m.WriteAAC(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			// group without IDR
			d = 4 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{1}, // non-IDR
			})
			require.NoError(t, err)

			d = 4500 * time.Millisecond
			err = m.WriteAAC(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			// group with IDR
			d = 6 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// group with IDR
			d = 7 * time.Second
			err = m.WriteH264(testTime.Add(d-1*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, err := ioutil.ReadAll(m.File("index.m3u8", "", "", "").Body)
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028,mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028,mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4,\n` +
					`(seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1,\n` +
					`(seg1\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" {
				dem := astits.NewDemuxer(context.Background(), m.File(ma[2], "", "", "").Body,
					astits.DemuxerOptPacketSize(188))

				// PMT
				pkt, err := dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					Header: &astits.PacketHeader{
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       0,
					},
					Payload: append([]byte{
						0x00, 0x00, 0xb0, 0x0d, 0x00, 0x00, 0xc1, 0x00,
						0x00, 0x00, 0x01, 0xf0, 0x00, 0x71, 0x10, 0xd8,
						0x78,
					}, bytes.Repeat([]byte{0xff}, 167)...),
				}, pkt)

				// PAT
				pkt, err = dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					Header: &astits.PacketHeader{
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       4096,
					},
					Payload: append([]byte{
						0x00, 0x02, 0xb0, 0x17, 0x00, 0x01, 0xc1, 0x00,
						0x00, 0xe1, 0x00, 0xf0, 0x00, 0x1b, 0xe1, 0x00,
						0xf0, 0x00, 0x0f, 0xe1, 0x01, 0xf0, 0x00, 0x2f,
						0x44, 0xb9, 0x9b,
					}, bytes.Repeat([]byte{0xff}, 157)...),
				}, pkt)

				// PES (H264)
				pkt, err = dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					AdaptationField: &astits.PacketAdaptationField{
						Length:                124,
						StuffingLength:        117,
						HasPCR:                true,
						PCR:                   &astits.ClockReference{},
						RandomAccessIndicator: true,
					},
					Header: &astits.PacketHeader{
						HasAdaptationField:        true,
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       256,
					},
					Payload: []byte{
						0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x80,
						0x05, 0x21, 0x00, 0x03, 0x19, 0x41, 0x00, 0x00,
						0x00, 0x01, 0x09, 0xf0, 0x00, 0x00, 0x00, 0x01,
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
						0x20, 0x00, 0x00, 0x00, 0x01, 0x08, 0x00, 0x00,
						0x00, 0x01, 0x05,
					},
				}, pkt)

				// PES (AAC)
				pkt, err = dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					AdaptationField: &astits.PacketAdaptationField{
						Length:                158,
						StuffingLength:        157,
						RandomAccessIndicator: true,
					},
					Header: &astits.PacketHeader{
						HasAdaptationField:        true,
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       257,
					},
					Payload: []byte{
						0x00, 0x00, 0x01, 0xc0, 0x00, 0x13, 0x80, 0x80,
						0x05, 0x21, 0x00, 0x07, 0xd8, 0x5f, 0xff, 0xf1,
						0x50, 0x80, 0x01, 0x7f, 0xfc, 0x01, 0x02, 0x03,
						0x04,
					},
				}, pkt)
			} else {
				byts, err := io.ReadAll(m.File("init.mp4", "", "", "").Body)
				require.NoError(t, err)

				boxes := []gomp4.BoxPath{
					{gomp4.BoxTypeFtyp()},
					{gomp4.BoxTypeMoov()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(), gomp4.BoxTypeVmhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(), gomp4.BoxTypeDinf()},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeAvcC(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeBtrt(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
					},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeSmhd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeEsds(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeBtrt(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
					},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
				}
				testMP4(t, byts, boxes)

				byts, err = io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)

				boxes = []gomp4.BoxPath{
					{gomp4.BoxTypeMoof()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeMfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
					{gomp4.BoxTypeMdat()},
				}
				testMP4(t, byts, boxes)
			}
		})
	}
}

func TestMuxerVideoOnly(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	for _, ca := range []string{
		"mpegts",
		"fmp4",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m, err := NewMuxer(v, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
			require.NoError(t, err)
			defer m.Close()

			// group with IDR
			d := 2 * time.Second
			err = m.WriteH264(testTime.Add(d-2*time.Second), d, [][]byte{
				testSPS, // SPS
				{8},     // PPS
				{5},     // IDR
			})
			require.NoError(t, err)

			// group with IDR
			d = 6 * time.Second
			err = m.WriteH264(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			// group with IDR
			d = 7 * time.Second
			err = m.WriteH264(testTime.Add(d-2*time.Second), d, [][]byte{
				{5}, // IDR
			})
			require.NoError(t, err)

			byts, err := ioutil.ReadAll(m.File("index.m3u8", "", "", "").Body)
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028\"\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.42c028\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4,\n` +
					`(seg0\.ts)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1,\n` +
					`(seg1\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:4\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:4.00000,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1.00000,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" { //nolint:dupl
				dem := astits.NewDemuxer(context.Background(), m.File(ma[2], "", "", "").Body,
					astits.DemuxerOptPacketSize(188))

				// PMT
				pkt, err := dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					Header: &astits.PacketHeader{
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       0,
					},
					Payload: append([]byte{
						0x00, 0x00, 0xb0, 0x0d, 0x00, 0x00, 0xc1, 0x00,
						0x00, 0x00, 0x01, 0xf0, 0x00, 0x71, 0x10, 0xd8,
						0x78,
					}, bytes.Repeat([]byte{0xff}, 167)...),
				}, pkt)

				// PAT
				pkt, err = dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					Header: &astits.PacketHeader{
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       4096,
					},
					Payload: append([]byte{
						0x00, 0x02, 0xb0, 0x12, 0x00, 0x01, 0xc1, 0x00,
						0x00, 0xe1, 0x00, 0xf0, 0x00, 0x1b, 0xe1, 0x00,
						0xf0, 0x00, 0x15, 0xbd, 0x4d, 0x56,
					}, bytes.Repeat([]byte{0xff}, 162)...),
				}, pkt)
			} else { //nolint:dupl
				byts, err := io.ReadAll(m.File("init.mp4", "", "", "").Body)
				require.NoError(t, err)

				boxes := []gomp4.BoxPath{
					{gomp4.BoxTypeFtyp()},
					{gomp4.BoxTypeMoov()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeVmhd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeAvcC(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeAvc1(), gomp4.BoxTypeBtrt(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
					},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
				}
				testMP4(t, byts, boxes)

				byts, err = io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)

				boxes = []gomp4.BoxPath{
					{gomp4.BoxTypeMoof()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeMfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
					{gomp4.BoxTypeMdat()},
				}
				testMP4(t, byts, boxes)
			}
		})
	}
}

func TestMuxerAudioOnly(t *testing.T) {
	audioTrack := &gortsplib.TrackMPEG4Audio{
		PayloadType: 97,
		Config: &mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}

	for _, ca := range []string{
		"mpegts",
		"fmp4",
	} {
		t.Run(ca, func(t *testing.T) {
			var v MuxerVariant
			if ca == "mpegts" {
				v = MuxerVariantMPEGTS
			} else {
				v = MuxerVariantFMP4
			}

			m, err := NewMuxer(v, 3, 1*time.Second, 0, 50*1024*1024, nil, audioTrack)
			require.NoError(t, err)
			defer m.Close()

			for i := 0; i < 100; i++ {
				d := 1 * time.Second
				err = m.WriteAAC(testTime.Add(d-1*time.Second), d, []byte{
					0x01, 0x02, 0x03, 0x04,
				})
				require.NoError(t, err)
			}

			d := 2 * time.Second
			err = m.WriteAAC(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			d = 3 * time.Second
			err = m.WriteAAC(testTime.Add(d-1*time.Second), d, []byte{
				0x01, 0x02, 0x03, 0x04,
			})
			require.NoError(t, err)

			byts, err := ioutil.ReadAll(m.File("index.m3u8", "", "", "").Body)
			require.NoError(t, err)

			if ca == "mpegts" {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:3\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			} else {
				require.Equal(t, "#EXTM3U\n"+
					"#EXT-X-VERSION:9\n"+
					"#EXT-X-INDEPENDENT-SEGMENTS\n"+
					"\n"+
					"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"mp4a.40.2\"\n"+
					"stream.m3u8\n", string(byts))
			}

			byts, err = ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
			require.NoError(t, err)

			var ma []string
			if ca == "mpegts" {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:3\n` +
					`#EXT-X-ALLOW-CACHE:NO\n` +
					`#EXT-X-TARGETDURATION:1\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:1,\n` +
					`(seg0\.ts)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			} else {
				re := regexp.MustCompile(`^#EXTM3U\n` +
					`#EXT-X-VERSION:9\n` +
					`#EXT-X-TARGETDURATION:2\n` +
					`#EXT-X-MEDIA-SEQUENCE:0\n` +
					`#EXT-X-MAP:URI="init.mp4"\n` +
					`\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:2.32200,\n` +
					`(seg0\.mp4)\n` +
					`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
					`#EXTINF:0.02322,\n` +
					`(seg1\.mp4)\n$`)
				ma = re.FindStringSubmatch(string(byts))
			}
			require.NotEqual(t, 0, len(ma))

			if ca == "mpegts" { //nolint:dupl
				dem := astits.NewDemuxer(context.Background(), m.File(ma[2], "", "", "").Body,
					astits.DemuxerOptPacketSize(188))

				// PMT
				pkt, err := dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					Header: &astits.PacketHeader{
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       0,
					},
					Payload: append([]byte{
						0x00, 0x00, 0xb0, 0x0d, 0x00, 0x00, 0xc1, 0x00,
						0x00, 0x00, 0x01, 0xf0, 0x00, 0x71, 0x10, 0xd8,
						0x78,
					}, bytes.Repeat([]byte{0xff}, 167)...),
				}, pkt)

				// PAT
				pkt, err = dem.NextPacket()
				require.NoError(t, err)
				require.Equal(t, &astits.Packet{
					Header: &astits.PacketHeader{
						HasPayload:                true,
						PayloadUnitStartIndicator: true,
						PID:                       4096,
					},
					Payload: append([]byte{
						0x00, 0x02, 0xb0, 0x12, 0x00, 0x01, 0xc1, 0x00,
						0x00, 0xe1, 0x01, 0xf0, 0x00, 0x0f, 0xe1, 0x01,
						0xf0, 0x00, 0xec, 0xe2, 0xb0, 0x94,
					}, bytes.Repeat([]byte{0xff}, 162)...),
				}, pkt)
			} else { //nolint:dupl
				byts, err := io.ReadAll(m.File("init.mp4", "", "", "").Body)
				require.NoError(t, err)

				boxes := []gomp4.BoxPath{
					{gomp4.BoxTypeFtyp()},
					{gomp4.BoxTypeMoov()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeTkhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMdhd()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeHdlr()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf()},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeSmhd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeDinf(), gomp4.BoxTypeDref(), gomp4.BoxTypeUrl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeEsds(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsd(), gomp4.BoxTypeMp4a(), gomp4.BoxTypeBtrt(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStts(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsc(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStsz(),
					},
					{
						gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(), gomp4.BoxTypeMinf(),
						gomp4.BoxTypeStbl(), gomp4.BoxTypeStco(),
					},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex()},
					{gomp4.BoxTypeMoov(), gomp4.BoxTypeMvex(), gomp4.BoxTypeTrex()},
				}
				testMP4(t, byts, boxes)

				byts, err = io.ReadAll(m.File(ma[2], "", "", "").Body)
				require.NoError(t, err)

				boxes = []gomp4.BoxPath{
					{gomp4.BoxTypeMoof()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeMfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfhd()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTfdt()},
					{gomp4.BoxTypeMoof(), gomp4.BoxTypeTraf(), gomp4.BoxTypeTrun()},
					{gomp4.BoxTypeMdat()},
				}
				testMP4(t, byts, boxes)
			}
		})
	}
}

func TestMuxerCloseBeforeFirstSegmentReader(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)

	// group with IDR
	err = m.WriteH264(testTime, 2*time.Second, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	m.Close()

	b := m.File("stream.m3u8", "", "", "").Body
	require.Equal(t, nil, b)
}

func TestMuxerMaxSegmentSize(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 0, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testTime, 2*time.Second, [][]byte{
		testSPS,
		{5}, // IDR
	})
	require.EqualError(t, err, "reached maximum segment size")
}

func TestMuxerDoubleRead(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         testSPS,
		PPS:         []byte{0x08},
	}

	m, err := NewMuxer(MuxerVariantMPEGTS, 3, 1*time.Second, 0, 50*1024*1024, videoTrack, nil)
	require.NoError(t, err)
	defer m.Close()

	err = m.WriteH264(testTime, 0, [][]byte{
		testSPS,
		{5}, // IDR
		{1},
	})
	require.NoError(t, err)

	err = m.WriteH264(testTime, 2*time.Second, [][]byte{
		{5}, // IDR
		{2},
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.File("stream.m3u8", "", "", "").Body)
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`\n` +
		`#EXT-X-PROGRAM-DATE-TIME:(.*?)\n` +
		`#EXTINF:2,\n` +
		`(seg0\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, 0, len(ma))

	byts1, err := ioutil.ReadAll(m.File(ma[2], "", "", "").Body)
	require.NoError(t, err)

	byts2, err := ioutil.ReadAll(m.File(ma[2], "", "", "").Body)
	require.NoError(t, err)
	require.Equal(t, byts1, byts2)
}
