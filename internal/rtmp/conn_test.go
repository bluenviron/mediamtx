package rtmp

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	nh264 "github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"
)

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)

func splitPath(u *url.URL) (app, stream string) {
	nu := *u
	nu.ForceQuery = false

	pathsegs := strings.Split(nu.RequestURI(), "/")
	if len(pathsegs) == 2 {
		app = pathsegs[1]
	}
	if len(pathsegs) == 3 {
		app = pathsegs[1]
		stream = pathsegs[2]
	}
	if len(pathsegs) > 3 {
		app = strings.Join(pathsegs[1:3], "/")
		stream = strings.Join(pathsegs[3:], "/")
	}
	return
}

func hsMakeDigest(key []byte, src []byte, gap int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := 0; i < 4; i++ {
		pos += int(p[base+i])
	}
	pos = (pos % 728) + base + 4
	return
}

func hsFindDigest(p []byte, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}
	return gap
}

func hsParse1(p []byte, peerkey []byte, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return
		}
	}
	ok = true
	digest = hsMakeDigest(key, p[pos:pos+32], -1)
	return
}

func writeHandshakeC2(w io.Writer, s0s1s2 []byte) error {
	ok, key := hsParse1(s0s1s2[1:1537], hsServerPartialKey, hsClientFullKey)
	if !ok {
		panic("unable to parse s0s1s2")
	}

	buf := make([]byte, 1536)
	rand.Read(buf)
	gap := len(buf) - 32
	digest := hsMakeDigest(key, buf, gap)
	copy(buf[gap:], digest)
	_, err := w.Write(buf)
	return err
}

func writeHandshakeC0C1(w io.Writer) error {
	buf := make([]byte, 1537)
	buf[0] = 0x03
	copy(buf[1:5], []byte{0x00, 0x00, 0x00, 0x00})
	copy(buf[5:9], []byte{0x09, 0x00, 0x7c, 0x02})
	rand.Read(buf[1+8:])
	gap := hsCalcDigestPos(buf[1:], 8)
	digest := hsMakeDigest(hsClientPartialKey, buf[1:], gap)
	copy(buf[gap+1:], digest)

	_, err := w.Write(buf)
	return err
}

func getTcURL(u string) string {
	ur, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	app, _ := splitPath(ur)
	nu := *ur
	nu.RawQuery = ""
	nu.Path = "/"
	return nu.String() + app
}

type chunk0 struct {
	chunkStreamID byte
	timestamp     uint32
	typ           byte
	streamID      uint32
	bodyLen       uint32
	body          []byte
}

func (m *chunk0) read(r io.Reader, chunkMaxBodyLen int) error {
	header := make([]byte, 12)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	if header[0]>>6 != 0 {
		return fmt.Errorf("wrong chunk header type")
	}

	m.chunkStreamID = header[0] & 0x3F
	m.timestamp = uint32(header[3])<<16 | uint32(header[2])<<8 | uint32(header[1])
	m.bodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	m.typ = header[7]
	m.streamID = uint32(header[8])<<24 | uint32(header[9])<<16 | uint32(header[10])<<8 | uint32(header[11])

	chunkBodyLen := int(m.bodyLen)
	if chunkBodyLen > chunkMaxBodyLen {
		chunkBodyLen = chunkMaxBodyLen
	}

	m.body = make([]byte, chunkBodyLen)
	_, err = r.Read(m.body)
	return err
}

func (m chunk0) write(w io.Writer) error {
	header := make([]byte, 12)
	header[0] = m.chunkStreamID
	header[1] = byte(m.timestamp >> 16)
	header[2] = byte(m.timestamp >> 8)
	header[3] = byte(m.timestamp)
	header[4] = byte(m.bodyLen >> 16)
	header[5] = byte(m.bodyLen >> 8)
	header[6] = byte(m.bodyLen)
	header[7] = m.typ
	header[8] = byte(m.streamID)
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(m.body)
	return err
}

type chunk1 struct {
	chunkStreamID byte
	typ           byte
	body          []byte
}

func (m chunk1) write(w io.Writer) error {
	header := make([]byte, 8)
	header[0] = 1<<6 | m.chunkStreamID
	l := uint32(len(m.body))
	header[4] = byte(l >> 16)
	header[5] = byte(l >> 8)
	header[6] = byte(l)
	header[7] = m.typ
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(m.body)
	return err
}

type chunk3 struct {
	chunkStreamID byte
	body          []byte
}

func (m chunk3) write(w io.Writer) error {
	header := make([]byte, 1)
	header[0] = 3<<6 | m.chunkStreamID
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(m.body)
	return err
}

func TestReadTracks(t *testing.T) {
	sps := []byte{
		0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
		0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
		0x00, 0x03, 0x00, 0x3d, 0x08,
	}

	pps := []byte{
		0x68, 0xee, 0x3c, 0x80,
	}

	for _, ca := range []string{
		"standard",
		"metadata without codec id",
		"no metadata",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				conn, err := ln.Accept()
				require.NoError(t, err)
				defer conn.Close()

				rconn := NewServerConn(conn)
				err = rconn.ServerHandshake()
				require.NoError(t, err)

				videoTrack, audioTrack, err := rconn.ReadTracks()
				require.NoError(t, err)

				switch ca {
				case "standard":
					videoTrack2, err := gortsplib.NewTrackH264(96, sps, pps, nil)
					require.NoError(t, err)
					require.Equal(t, videoTrack2, videoTrack)

					audioTrack2, err := gortsplib.NewTrackAAC(96, 2, 44100, 2, nil, 13, 3, 3)
					require.NoError(t, err)
					require.Equal(t, audioTrack2, audioTrack)

				case "metadata without codec id":
					videoTrack2, err := gortsplib.NewTrackH264(96, sps, pps, nil)
					require.NoError(t, err)
					require.Equal(t, videoTrack2, videoTrack)

					require.Equal(t, (*gortsplib.TrackAAC)(nil), audioTrack)

				case "no metadata":
					videoTrack2, err := gortsplib.NewTrackH264(96, sps, pps, nil)
					require.NoError(t, err)
					require.Equal(t, videoTrack2, videoTrack)

					require.Equal(t, (*gortsplib.TrackAAC)(nil), audioTrack)
				}

				close(done)
			}()

			conn, err := net.Dial("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer conn.Close()

			// C->S handshake C0+C1
			err = writeHandshakeC0C1(conn)
			require.NoError(t, err)

			// S->C handshake S0+S1+S2
			s0s1s2 := make([]byte, 1536*2+1)
			_, err = conn.Read(s0s1s2)
			require.NoError(t, err)

			// C->S handshake C2
			err = writeHandshakeC2(conn, s0s1s2)
			require.NoError(t, err)

			// C->S connect
			byts := flvio.FillAMF0ValsMalloc([]interface{}{
				"connect",
				1,
				flvio.AMFMap{
					{K: "app", V: "/stream"},
					{K: "flashVer", V: "LNX 9,0,124,2"},
					{K: "tcUrl", V: getTcURL("rtmp://127.0.0.1:9121/stream")},
					{K: "fpad", V: false},
					{K: "capabilities", V: 15},
					{K: "audioCodecs", V: 4071},
					{K: "videoCodecs", V: 252},
					{K: "videoFunction", V: 1},
				},
			})
			err = chunk0{
				chunkStreamID: 3,
				typ:           0x14,
				bodyLen:       uint32(len(byts)),
				body:          byts[:128],
			}.write(conn)
			require.NoError(t, err)
			err = chunk3{
				chunkStreamID: 3,
				body:          byts[128:],
			}.write(conn)
			require.NoError(t, err)

			// S->C window acknowledgement size
			var c0 chunk0
			err = c0.read(conn, 128)
			require.NoError(t, err)
			require.Equal(t, chunk0{
				chunkStreamID: 2,
				typ:           5,
				bodyLen:       4,
				body:          []byte{0x00, 38, 37, 160},
			}, c0)

			// S->C set peer bandwidth
			err = c0.read(conn, 128)
			require.NoError(t, err)
			require.Equal(t, chunk0{
				chunkStreamID: 2,
				typ:           6,
				bodyLen:       5,
				body:          []byte{0x00, 0x26, 0x25, 0xa0, 0x02},
			}, c0)

			// S->C set chunk size
			err = c0.read(conn, 128)
			require.NoError(t, err)
			require.Equal(t, chunk0{
				chunkStreamID: 2,
				typ:           1,
				bodyLen:       4,
				body:          []byte{0x00, 0x01, 0x00, 0x00},
			}, c0)

			// S->C result
			err = c0.read(conn, 65536)
			require.NoError(t, err)
			require.Equal(t, uint8(3), c0.chunkStreamID)
			require.Equal(t, uint8(0x14), c0.typ)
			arr, err := flvio.ParseAMFVals(c0.body, false)
			require.NoError(t, err)
			require.Equal(t, []interface{}{
				"_result",
				float64(1),
				flvio.AMFMap{
					{K: "fmsVer", V: "LNX 9,0,124,2"},
					{K: "capabilities", V: float64(31)},
				},
				flvio.AMFMap{
					{K: "level", V: "status"},
					{K: "code", V: "NetConnection.Connect.Success"},
					{K: "description", V: "Connection succeeded."},
					{K: "objectEncoding", V: float64(0)},
				},
			}, arr)

			// C->S set chunk size
			err = chunk0{
				chunkStreamID: 2,
				typ:           1,
				bodyLen:       4,
				body:          []byte{0x00, 0x01, 0x00, 0x00},
			}.write(conn)
			require.NoError(t, err)

			// C->S releaseStream
			err = chunk1{
				chunkStreamID: 3,
				typ:           0x14,
				body: flvio.FillAMF0ValsMalloc([]interface{}{
					"releaseStream",
					float64(2),
					nil,
					"",
				}),
			}.write(conn)
			require.NoError(t, err)

			// C->S FCPublish
			err = chunk1{
				chunkStreamID: 3,
				typ:           0x14,
				body: flvio.FillAMF0ValsMalloc([]interface{}{
					"FCPublish",
					float64(3),
					nil,
					"",
				}),
			}.write(conn)
			require.NoError(t, err)

			// C->S createStream
			err = chunk3{
				chunkStreamID: 3,
				body: flvio.FillAMF0ValsMalloc([]interface{}{
					"createStream",
					float64(4),
					nil,
				}),
			}.write(conn)
			require.NoError(t, err)

			// S->C result
			err = c0.read(conn, 65536)
			require.NoError(t, err)
			require.Equal(t, uint8(3), c0.chunkStreamID)
			require.Equal(t, uint8(0x14), c0.typ)
			arr, err = flvio.ParseAMFVals(c0.body, false)
			require.NoError(t, err)
			require.Equal(t, []interface{}{
				"_result",
				float64(4),
				nil,
				float64(1),
			}, arr)

			// C->S publish
			byts = flvio.FillAMF0ValsMalloc([]interface{}{
				"publish",
				float64(5),
				nil,
				"",
				"live",
			})
			err = chunk0{
				chunkStreamID: 8,
				typ:           0x14,
				streamID:      1,
				bodyLen:       uint32(len(byts)),
				body:          byts,
			}.write(conn)
			require.NoError(t, err)

			// S->C onStatus
			err = c0.read(conn, 65536)
			require.NoError(t, err)
			require.Equal(t, uint8(5), c0.chunkStreamID)
			require.Equal(t, uint8(0x14), c0.typ)
			arr, err = flvio.ParseAMFVals(c0.body, false)
			require.NoError(t, err)
			require.Equal(t, []interface{}{
				"onStatus",
				float64(5),
				nil,
				flvio.AMFMap{
					{K: "level", V: "status"},
					{K: "code", V: "NetStream.Publish.Start"},
					{K: "description", V: "publish start"},
				},
			}, arr)

			switch ca {
			case "standard":
				// C->S metadata
				byts = flvio.FillAMF0ValsMalloc([]interface{}{
					"@setDataFrame",
					"onMetaData",
					flvio.AMFMap{
						{
							K: "videodatarate",
							V: float64(0),
						},
						{
							K: "videocodecid",
							V: float64(codecH264),
						},
						{
							K: "audiodatarate",
							V: float64(0),
						},
						{
							K: "audiocodecid",
							V: float64(codecAAC),
						},
					},
				})
				err = chunk0{
					chunkStreamID: 4,
					typ:           0x12,
					streamID:      1,
					bodyLen:       uint32(len(byts)),
					body:          byts,
				}.write(conn)
				require.NoError(t, err)

				// C->S H264 decoder config
				codec := nh264.Codec{
					SPS: map[int][]byte{
						0: sps,
					},
					PPS: map[int][]byte{
						0: pps,
					},
				}
				b := make([]byte, 128)
				var n int
				codec.ToConfig(b, &n)
				body := append([]byte{flvio.FRAME_KEY<<4 | flvio.VIDEO_H264, 0, 0, 0, 0}, b[:n]...)
				err = chunk0{
					chunkStreamID: 6,
					typ:           flvio.TAG_VIDEO,
					streamID:      1,
					bodyLen:       uint32(len(body)),
					body:          body,
				}.write(conn)
				require.NoError(t, err)

				// C->S AAC decoder config
				enc, err := aac.MPEG4AudioConfig{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Encode()
				require.NoError(t, err)
				err = chunk0{
					chunkStreamID: 4,
					typ:           flvio.TAG_AUDIO,
					streamID:      1,
					bodyLen:       uint32(len(enc) + 2),
					body: append([]byte{
						flvio.SOUND_AAC<<4 | flvio.SOUND_44Khz<<2 | flvio.SOUND_16BIT<<1 | flvio.SOUND_STEREO,
						flvio.AAC_SEQHDR,
					}, enc...),
				}.write(conn)
				require.NoError(t, err)

			case "metadata without codec id":
				// C->S metadata
				byts = flvio.FillAMF0ValsMalloc([]interface{}{
					"@setDataFrame",
					"onMetaData",
					flvio.AMFMap{
						{
							K: "width",
							V: float64(2688),
						},
						{
							K: "height",
							V: float64(1520),
						},
						{
							K: "framerate",
							V: float64(0o25),
						},
					},
				})
				err = chunk0{
					chunkStreamID: 4,
					typ:           0x12,
					streamID:      1,
					bodyLen:       uint32(len(byts)),
					body:          byts,
				}.write(conn)
				require.NoError(t, err)

				// C->S H264 decoder config
				codec := nh264.Codec{
					SPS: map[int][]byte{
						0: sps,
					},
					PPS: map[int][]byte{
						0: pps,
					},
				}
				b := make([]byte, 128)
				var n int
				codec.ToConfig(b, &n)
				body := append([]byte{flvio.FRAME_KEY<<4 | flvio.VIDEO_H264, 0, 0, 0, 0}, b[:n]...)
				err = chunk0{
					chunkStreamID: 6,
					typ:           flvio.TAG_VIDEO,
					streamID:      1,
					bodyLen:       uint32(len(body)),
					body:          body,
				}.write(conn)
				require.NoError(t, err)

			case "no metadata":
				// C->S H264 decoder config
				codec := nh264.Codec{
					SPS: map[int][]byte{
						0: sps,
					},
					PPS: map[int][]byte{
						0: pps,
					},
				}
				b := make([]byte, 128)
				var n int
				codec.ToConfig(b, &n)
				body := append([]byte{flvio.FRAME_KEY<<4 | flvio.VIDEO_H264, 0, 0, 0, 0}, b[:n]...)
				err = chunk0{
					chunkStreamID: 6,
					typ:           flvio.TAG_VIDEO,
					streamID:      1,
					bodyLen:       uint32(len(body)),
					body:          body,
				}.write(conn)
				require.NoError(t, err)
			}

			<-done
		})
	}
}

func TestWriteTracks(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9121")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		require.NoError(t, err)
		defer conn.Close()

		rconn := NewServerConn(conn)
		err = rconn.ServerHandshake()
		require.NoError(t, err)

		videoTrack, err := gortsplib.NewTrackH264(96,
			[]byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			[]byte{
				0x68, 0xee, 0x3c, 0x80,
			},
			nil)
		require.NoError(t, err)

		audioTrack, err := gortsplib.NewTrackAAC(96, 2, 44100, 2, nil, 13, 3, 3)
		require.NoError(t, err)

		err = rconn.WriteTracks(videoTrack, audioTrack)
		require.NoError(t, err)
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:9121")
	require.NoError(t, err)
	defer conn.Close()

	// C->S handshake C0+C1
	err = writeHandshakeC0C1(conn)
	require.NoError(t, err)

	// S->C handshake S0+S1+S2
	s0s1s2 := make([]byte, 1536*2+1)
	_, err = conn.Read(s0s1s2)
	require.NoError(t, err)

	// C->S handshake C2
	err = writeHandshakeC2(conn, s0s1s2)
	require.NoError(t, err)

	// C->S connect
	byts := flvio.FillAMF0ValsMalloc([]interface{}{
		"connect",
		1,
		flvio.AMFMap{
			{K: "app", V: "/stream"},
			{K: "flashVer", V: "LNX 9,0,124,2"},
			{K: "tcUrl", V: getTcURL("rtmp://127.0.0.1:9121/stream")},
			{K: "fpad", V: false},
			{K: "capabilities", V: 15},
			{K: "audioCodecs", V: 4071},
			{K: "videoCodecs", V: 252},
			{K: "videoFunction", V: 1},
		},
	})
	err = chunk0{
		chunkStreamID: 3,
		typ:           0x14,
		bodyLen:       uint32(len(byts)),
		body:          byts[:128],
	}.write(conn)
	require.NoError(t, err)
	err = chunk3{
		chunkStreamID: 3,
		body:          byts[128:],
	}.write(conn)
	require.NoError(t, err)

	// S->C window acknowledgement size
	var c0 chunk0
	err = c0.read(conn, 128)
	require.NoError(t, err)
	require.Equal(t, chunk0{
		chunkStreamID: 2,
		typ:           5,
		bodyLen:       4,
		body:          []byte{0x00, 38, 37, 160},
	}, c0)

	// S->C set peer bandwidth
	err = c0.read(conn, 128)
	require.NoError(t, err)
	require.Equal(t, chunk0{
		chunkStreamID: 2,
		typ:           6,
		bodyLen:       5,
		body:          []byte{0x00, 0x26, 0x25, 0xa0, 0x02},
	}, c0)

	// S->C set chunk size
	err = c0.read(conn, 128)
	require.NoError(t, err)
	require.Equal(t, chunk0{
		chunkStreamID: 2,
		typ:           1,
		bodyLen:       4,
		body:          []byte{0x00, 0x01, 0x00, 0x00},
	}, c0)

	// S->C result
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(3), c0.chunkStreamID)
	require.Equal(t, uint8(0x14), c0.typ)
	arr, err := flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"_result",
		float64(1),
		flvio.AMFMap{
			{K: "fmsVer", V: "LNX 9,0,124,2"},
			{K: "capabilities", V: float64(31)},
		},
		flvio.AMFMap{
			{K: "level", V: "status"},
			{K: "code", V: "NetConnection.Connect.Success"},
			{K: "description", V: "Connection succeeded."},
			{K: "objectEncoding", V: float64(0)},
		},
	}, arr)

	// C->S window acknowledgement size
	err = chunk0{
		chunkStreamID: 2,
		typ:           0x05,
		bodyLen:       4,
		body:          []byte{0x00, 0x26, 0x25, 0xa0},
	}.write(conn)
	require.NoError(t, err)

	// C->S set chunk size
	err = chunk0{
		chunkStreamID: 2,
		typ:           1,
		bodyLen:       4,
		body:          []byte{0x00, 0x01, 0x00, 0x00},
	}.write(conn)
	require.NoError(t, err)

	// C->S createStream
	err = chunk1{
		chunkStreamID: 3,
		typ:           0x14,
		body: flvio.FillAMF0ValsMalloc([]interface{}{
			"createStream",
			float64(2),
			nil,
		}),
	}.write(conn)
	require.NoError(t, err)

	// S->C result
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(3), c0.chunkStreamID)
	require.Equal(t, uint8(0x14), c0.typ)
	arr, err = flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"_result",
		float64(2),
		nil,
		float64(1),
	}, arr)

	// C->S getStreamLength
	byts = flvio.FillAMF0ValsMalloc([]interface{}{
		"getStreamLength",
		float64(3),
		nil,
		"",
	})
	err = chunk0{
		chunkStreamID: 8,
		bodyLen:       uint32(len(byts)),
		body:          byts,
	}.write(conn)
	require.NoError(t, err)

	// C->S play
	byts = flvio.FillAMF0ValsMalloc([]interface{}{
		"play",
		float64(4),
		nil,
		"",
		float64(-2000),
	})
	err = chunk0{
		chunkStreamID: 8,
		typ:           0x14,
		bodyLen:       uint32(len(byts)),
		body:          byts,
	}.write(conn)
	require.NoError(t, err)

	// S->C event "stream is recorded"
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, chunk0{
		chunkStreamID: 2,
		typ:           4,
		bodyLen:       6,
		body:          []byte{0x00, 0x04, 0x00, 0x00, 0x00, 0x01},
	}, c0)

	// S->C event "stream begin 1"
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, chunk0{
		chunkStreamID: 2,
		typ:           4,
		bodyLen:       6,
		body:          []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
	}, c0)

	// S->C onStatus
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(5), c0.chunkStreamID)
	require.Equal(t, uint8(0x14), c0.typ)
	arr, err = flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"onStatus",
		float64(4),
		nil,
		flvio.AMFMap{
			{K: "level", V: "status"},
			{K: "code", V: "NetStream.Play.Reset"},
			{K: "description", V: "play reset"},
		},
	}, arr)

	// S->C onStatus
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(5), c0.chunkStreamID)
	require.Equal(t, uint8(0x14), c0.typ)
	arr, err = flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"onStatus",
		float64(4),
		nil,
		flvio.AMFMap{
			{K: "level", V: "status"},
			{K: "code", V: "NetStream.Play.Start"},
			{K: "description", V: "play start"},
		},
	}, arr)

	// S->C onStatus
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(5), c0.chunkStreamID)
	require.Equal(t, uint8(0x14), c0.typ)
	arr, err = flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"onStatus",
		float64(4),
		nil,
		flvio.AMFMap{
			{K: "level", V: "status"},
			{K: "code", V: "NetStream.Data.Start"},
			{K: "description", V: "data start"},
		},
	}, arr)

	// S->C onStatus
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(5), c0.chunkStreamID)
	require.Equal(t, uint8(0x14), c0.typ)
	arr, err = flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"onStatus",
		float64(4),
		nil,
		flvio.AMFMap{
			{K: "level", V: "status"},
			{K: "code", V: "NetStream.Play.PublishNotify"},
			{K: "description", V: "publish notify"},
		},
	}, arr)

	// S->C onMetadata
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(4), c0.chunkStreamID)
	require.Equal(t, uint8(0x12), c0.typ)
	arr, err = flvio.ParseAMFVals(c0.body, false)
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		"onMetaData",
		flvio.AMFMap{
			{K: "videodatarate", V: float64(0)},
			{K: "videocodecid", V: float64(7)},
			{K: "audiodatarate", V: float64(0)},
			{K: "audiocodecid", V: float64(10)},
		},
	}, arr)

	// S->C H264 decoder config
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(6), c0.chunkStreamID)
	require.Equal(t, uint8(0x09), c0.typ)
	require.Equal(t, []byte{
		0x17, 0x0, 0x0, 0x0, 0x0, 0x1, 0x64, 0x0,
		0xc, 0xff, 0xe1, 0x0, 0x15, 0x67, 0x64, 0x0,
		0xc, 0xac, 0x3b, 0x50, 0xb0, 0x4b, 0x42, 0x0,
		0x0, 0x3, 0x0, 0x2, 0x0, 0x0, 0x3, 0x0,
		0x3d, 0x8, 0x1, 0x0, 0x4, 0x68, 0xee, 0x3c,
		0x80,
	}, c0.body)

	// S->C AAC decoder config
	err = c0.read(conn, 65536)
	require.NoError(t, err)
	require.Equal(t, uint8(4), c0.chunkStreamID)
	require.Equal(t, uint8(0x08), c0.typ)
	require.Equal(t, []byte{0xae, 0x0, 0x12, 0x10}, c0.body)
}
