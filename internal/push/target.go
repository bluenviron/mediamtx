// Package push contains push target implementations.
package push

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/bytecounter"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	mtls "github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	retryPause   = 5 * time.Second
	encodingAMF0 = 0
	fmleFlashVer = "FMLE/3.0 (compatible; mediamtx)"
)

type fmleRTMPClient struct {
	nconn net.Conn
	bc    *bytecounter.ReadWriter
	mrw   *message.ReadWriter
}

func splitPath(u *url.URL) (string, string) {
	pathsegs := strings.Split(u.Path, "/")

	var app string
	var streamKey string

	switch {
	case len(pathsegs) == 2:
		app = pathsegs[1]

	case len(pathsegs) == 3:
		app = pathsegs[1]
		streamKey = pathsegs[2]

	case len(pathsegs) > 3:
		app = strings.Join(pathsegs[1:3], "/")
		streamKey = strings.Join(pathsegs[3:], "/")
	}

	return app, streamKey
}

func getTcURL(u *url.URL) string {
	app, _ := splitPath(u)
	nu, _ := url.Parse(u.String())
	nu.RawQuery = ""
	nu.Path = "/"
	return nu.String() + app
}

func readCommandResult(mrw *message.ReadWriter, commandID int) (*message.CommandAMF0, error) {
	for {
		msg, err := mrw.Read()
		if err != nil {
			return nil, err
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID || (cmd.CommandID == 0 &&
				(cmd.Name == "_result" || cmd.Name == "_error")) {
				return cmd, nil
			}
		}
	}
}

func resultIsOK2(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	v, ok := res.Arguments[1].(float64)
	if !ok {
		return false
	}

	return v == 1
}

func objectOrArray(v interface{}) (amf0.Object, bool) {
	switch vv := v.(type) {
	case amf0.Object:
		return vv, true
	case amf0.ECMAArray:
		return amf0.Object(vv), true
	}
	return nil, false
}

func resultIsOK1(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}

	ma, ok := objectOrArray(res.Arguments[1])
	if !ok {
		return false
	}

	v, ok := ma.Get("level")
	if !ok {
		return false
	}

	return v == "status"
}

func newFMLERTMPClient(ctx context.Context, u *url.URL, tlsConfig *tls.Config) (*fmleRTMPClient, error) {
	var nconn net.Conn
	var err error

	if u.Scheme == "rtmp" {
		dialer := &net.Dialer{}
		nconn, err = dialer.DialContext(ctx, "tcp", u.Host)
	} else {
		dialer := &tls.Dialer{Config: tlsConfig}
		nconn, err = dialer.DialContext(ctx, "tcp", u.Host)
	}
	if err != nil {
		return nil, err
	}

	closerDone := make(chan struct{})
	closerTerminate := make(chan struct{})

	go func() {
		defer close(closerDone)
		select {
		case <-closerTerminate:
		case <-ctx.Done():
			nconn.Close()
		}
	}()

	c := &fmleRTMPClient{nconn: nconn}

	err = c.initialize(u)
	close(closerTerminate)
	<-closerDone

	if err != nil {
		nconn.Close()
		return nil, err
	}

	return c, nil
}

func (c *fmleRTMPClient) initialize(u *url.URL) error {
	c.bc = bytecounter.NewReadWriter(c.nconn)

	_, _, err := handshake.DoClient(c.bc, false, false)
	if err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	c.mrw = message.NewReadWriter(c.bc, c.bc, false)

	err = c.mrw.Write(&message.SetWindowAckSize{Value: 2500000})
	if err != nil {
		return fmt.Errorf("SetWindowAckSize failed: %w", err)
	}

	err = c.mrw.Write(&message.SetPeerBandwidth{Value: 2500000, Type: 2})
	if err != nil {
		return fmt.Errorf("SetPeerBandwidth failed: %w", err)
	}

	err = c.mrw.Write(&message.SetChunkSize{Value: 65536})
	if err != nil {
		return fmt.Errorf("SetChunkSize failed: %w", err)
	}

	app, streamKey := splitPath(u)
	tcURL := getTcURL(u)

	connectArg := amf0.Object{
		{Key: "app", Value: app},
		{Key: "flashVer", Value: fmleFlashVer},
		{Key: "tcUrl", Value: tcURL},
		{Key: "fpad", Value: false},
		{Key: "capabilities", Value: float64(15)},
		{Key: "audioCodecs", Value: float64(4071)},
		{Key: "videoCodecs", Value: float64(252)},
		{Key: "videoFunction", Value: float64(1)},
		{Key: "objectEncoding", Value: float64(encodingAMF0)},
		{Key: "type", Value: "nonprivate"},
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "connect",
		CommandID:     1,
		Arguments:     []any{connectArg},
	})
	if err != nil {
		return fmt.Errorf("connect command failed: %w", err)
	}

	res, err := readCommandResult(c.mrw, 1)
	if err != nil {
		return fmt.Errorf("connect result read failed: %w", err)
	}

	if res.Name == "_error" {
		return fmt.Errorf("connect rejected: %v", res.Arguments)
	}

	if res.Name != "_result" {
		return fmt.Errorf("unexpected connect result: %s", res.Name)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "releaseStream",
		CommandID:     2,
		Arguments:     []any{nil, streamKey},
	})
	if err != nil {
		return fmt.Errorf("releaseStream failed: %w", err)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "FCPublish",
		CommandID:     3,
		Arguments:     []any{nil, streamKey},
	})
	if err != nil {
		return fmt.Errorf("FCPublish failed: %w", err)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "createStream",
		CommandID:     4,
		Arguments:     []any{nil},
	})
	if err != nil {
		return fmt.Errorf("createStream failed: %w", err)
	}

	res, err = readCommandResult(c.mrw, 4)
	if err != nil {
		return fmt.Errorf("createStream result read failed: %w", err)
	}

	if res.Name != "_result" || !resultIsOK2(res) {
		return fmt.Errorf("createStream rejected: %v", res)
	}

	err = c.mrw.Write(&message.CommandAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Name:            "publish",
		CommandID:       5,
		Arguments:       []any{nil, streamKey, "live"},
	})
	if err != nil {
		return fmt.Errorf("publish command failed: %w", err)
	}

	for i := 0; i < 10; i++ {
		msg, err := c.mrw.Read()
		if err != nil {
			return fmt.Errorf("publish status read failed (attempt %d): %w", i+1, err)
		}

		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.Name == "onStatus" {
				if !resultIsOK1(cmd) {
					return fmt.Errorf("publish rejected: %v", cmd)
				}
				return nil
			}
			if cmd.Name == "_error" {
				return fmt.Errorf("publish error: %v", cmd.Arguments)
			}
		}
	}

	return fmt.Errorf("no publish response received after 10 attempts")
}

func (c *fmleRTMPClient) Close() {
	c.nconn.Close()
}

func (c *fmleRTMPClient) NetConn() net.Conn {
	return c.nconn
}

func (c *fmleRTMPClient) BytesReceived() uint64 {
	return c.bc.Reader.Count()
}

func (c *fmleRTMPClient) BytesSent() uint64 {
	return c.bc.Writer.Count()
}

func (c *fmleRTMPClient) Read() (message.Message, error) {
	return c.mrw.Read()
}

func (c *fmleRTMPClient) Write(msg message.Message) error {
	return c.mrw.Write(msg)
}

func multiplyAndDivide(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return secs*m + dec*m/d
}

func timestampToDuration(t int64, clockRate int) time.Duration {
	return multiplyAndDivide(time.Duration(t), time.Second, time.Duration(clockRate))
}

// countingWriter wraps an io.Writer and counts bytes written.
type countingWriter struct {
	w     io.Writer
	count *uint64
}

func (c *countingWriter) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	atomic.AddUint64(c.count, uint64(n))
	return n, err
}

type targetParent interface {
	logger.Writer
}

// Target is a push target.
type Target struct {
	URL          string
	ReadTimeout  conf.Duration
	WriteTimeout conf.Duration
	Parent       targetParent
	PathName     string

	ctx          context.Context
	ctxCancel    func()
	uuid         uuid.UUID
	created      time.Time
	mutex        sync.RWMutex
	state        defs.APIPushTargetState
	errorMsg     string
	bytesSent    uint64
	stream       *stream.Stream
	reader       *stream.Reader
	streamLoaded bool

	done chan struct{}
}

// Initialize initializes Target.
func (t *Target) Initialize() {
	t.ctx, t.ctxCancel = context.WithCancel(context.Background())
	t.uuid = uuid.New()
	t.created = time.Now()
	t.state = defs.APIPushTargetStateIdle
	t.done = make(chan struct{})

	t.Log(logger.Info, "created push target to %s", t.URL)

	go t.run()
}

// Close closes the Target.
func (t *Target) Close() {
	t.Log(logger.Info, "closing push target to %s", t.URL)
	t.ctxCancel()
	<-t.done
}

// Log implements logger.Writer.
func (t *Target) Log(level logger.Level, format string, args ...any) {
	t.Parent.Log(level, "[push %s] "+format, append([]any{t.uuid.String()[:8]}, args...)...)
}

// SetStream sets the stream to push.
func (t *Target) SetStream(strm *stream.Stream) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.stream = strm
	t.streamLoaded = true
}

// ClearStream clears the stream.
func (t *Target) ClearStream() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.stream = nil
	t.streamLoaded = false
}

// APIItem returns the API item.
func (t *Target) APIItem() *defs.APIPushTarget {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return &defs.APIPushTarget{
		ID:        t.uuid,
		Created:   t.created,
		URL:       t.URL,
		State:     t.state,
		Error:     t.errorMsg,
		BytesSent: atomic.LoadUint64(&t.bytesSent),
	}
}

// UUID returns the target UUID.
func (t *Target) UUID() uuid.UUID {
	return t.uuid
}

func (t *Target) run() {
	defer close(t.done)

	for {
		ok := t.runInner()
		if !ok {
			return
		}

		select {
		case <-time.After(retryPause):
		case <-t.ctx.Done():
			return
		}
	}
}

func (t *Target) runInner() bool {
	// Wait for stream to be available
	for {
		t.mutex.RLock()
		strm := t.stream
		loaded := t.streamLoaded
		t.mutex.RUnlock()

		if loaded && strm != nil {
			break
		}

		if loaded && strm == nil {
			t.mutex.Lock()
			t.state = defs.APIPushTargetStateIdle
			t.mutex.Unlock()
		}

		select {
		case <-time.After(500 * time.Millisecond):
		case <-t.ctx.Done():
			return false
		}
	}

	t.mutex.Lock()
	t.state = defs.APIPushTargetStateRunning
	t.errorMsg = ""
	t.mutex.Unlock()

	var err error

	switch {
	case strings.HasPrefix(t.URL, "rtmp://") || strings.HasPrefix(t.URL, "rtmps://"):
		err = t.runRTMP()
	case strings.HasPrefix(t.URL, "rtsp://") || strings.HasPrefix(t.URL, "rtsps://"):
		err = t.runRTSP()
	case strings.HasPrefix(t.URL, "srt://"):
		err = t.runSRT()
	default:
		err = fmt.Errorf("unsupported protocol")
	}

	if err != nil {
		t.Log(logger.Error, "push error: %v", err)

		t.mutex.Lock()
		t.state = defs.APIPushTargetStateError
		t.errorMsg = err.Error()
		t.mutex.Unlock()
	}

	return true
}

func (t *Target) addBytesSent(n uint64) {
	atomic.AddUint64(&t.bytesSent, n)
}

func (t *Target) runRTMP() error {
	t.Log(logger.Debug, "connecting to RTMP server")

	// Resolve the URL with path variables
	targetURL := t.resolveURL()

	u, err := url.Parse(targetURL)
	if err != nil {
		return err
	}

	// Add default port
	_, _, err = net.SplitHostPort(u.Host)
	if err != nil {
		if u.Scheme == "rtmp" {
			u.Host = net.JoinHostPort(u.Host, "1935")
		} else {
			u.Host = net.JoinHostPort(u.Host, "1936")
		}
	}

	t.mutex.RLock()
	strm := t.stream
	t.mutex.RUnlock()

	if strm == nil {
		return fmt.Errorf("stream is not available")
	}

	// Create reader
	reader := &stream.Reader{
		Parent: t,
	}

	// Setup tracks
	var tracks []*gortmplib.Track
	var writer *gortmplib.Writer

	for _, media := range strm.Desc.Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.H265:
				vps, sps, pps := forma.SafeParams()
				track := &gortmplib.Track{
					Codec: &codecs.H265{
						VPS: vps,
						SPS: sps,
						PPS: pps,
					},
				}
				tracks = append(tracks, track)

				var videoDTSExtractor *h265.DTSExtractor

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if videoDTSExtractor == nil {
							if !h265.IsRandomAccess(u.Payload.(unit.PayloadH265)) {
								return nil
							}
							videoDTSExtractor = &h265.DTSExtractor{}
							videoDTSExtractor.Initialize()
						}

						dts, err := videoDTSExtractor.Extract(u.Payload.(unit.PayloadH265), u.PTS)
						if err != nil {
							return err
						}

						err = writer.WriteH265(
							track,
							timestampToDuration(u.PTS, forma.ClockRate()),
							timestampToDuration(dts, forma.ClockRate()),
							u.Payload.(unit.PayloadH265))
						if err != nil {
							return err
						}

						// Count bytes sent (approximate size of payload)
						for _, nalu := range u.Payload.(unit.PayloadH265) {
							t.addBytesSent(uint64(len(nalu)))
						}
						return nil
					})

			case *format.H264:
				sps, pps := forma.SafeParams()
				track := &gortmplib.Track{
					Codec: &codecs.H264{
						SPS: sps,
						PPS: pps,
					},
				}
				tracks = append(tracks, track)

				var videoDTSExtractor *h264.DTSExtractor

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						idrPresent := false
						nonIDRPresent := false

						for _, nalu := range u.Payload.(unit.PayloadH264) {
							typ := h264.NALUType(nalu[0] & 0x1F)
							switch typ {
							case h264.NALUTypeIDR:
								idrPresent = true
							case h264.NALUTypeNonIDR:
								nonIDRPresent = true
							}
						}

						if videoDTSExtractor == nil {
							if !idrPresent {
								return nil
							}
							videoDTSExtractor = &h264.DTSExtractor{}
							videoDTSExtractor.Initialize()
						} else if !idrPresent && !nonIDRPresent {
							return nil
						}

						dts, err := videoDTSExtractor.Extract(u.Payload.(unit.PayloadH264), u.PTS)
						if err != nil {
							return err
						}

						err = writer.WriteH264(
							track,
							timestampToDuration(u.PTS, forma.ClockRate()),
							timestampToDuration(dts, forma.ClockRate()),
							u.Payload.(unit.PayloadH264))
						if err != nil {
							return err
						}

						// Count bytes sent (approximate size of payload)
						for _, nalu := range u.Payload.(unit.PayloadH264) {
							t.addBytesSent(uint64(len(nalu)))
						}
						return nil
					})

			case *format.MPEG4Audio:
				track := &gortmplib.Track{
					Codec: &codecs.MPEG4Audio{
						Config: forma.Config,
					},
				}
				tracks = append(tracks, track)

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						for i, au := range u.Payload.(unit.PayloadMPEG4Audio) {
							pts := u.PTS + int64(i)*1024 // SamplesPerAccessUnit

							err := writer.WriteMPEG4Audio(
								track,
								timestampToDuration(pts, forma.ClockRate()),
								au,
							)
							if err != nil {
								return err
							}

							// Count bytes sent
							t.addBytesSent(uint64(len(au)))
						}

						return nil
					})
			}
		}
	}

	if len(tracks) == 0 {
		return fmt.Errorf("no supported tracks found for RTMP push")
	}

	connectCtx, connectCtxCancel := context.WithTimeout(t.ctx, 30*time.Second)
	conn, err := newFMLERTMPClient(connectCtx, u, mtls.MakeConfig(u.Hostname(), ""))
	connectCtxCancel()
	if err != nil {
		return err
	}

	defer conn.Close()

	t.Log(logger.Info, "connected to %s", targetURL)

	// Initialize writer
	writer = &gortmplib.Writer{
		Conn:   conn,
		Tracks: tracks,
	}
	err = writer.Initialize()
	if err != nil {
		return err
	}

	// Add reader to stream
	strm.AddReader(reader)
	defer strm.RemoveReader(reader)

	t.mutex.Lock()
	t.reader = reader
	t.mutex.Unlock()

	conn.NetConn().SetReadDeadline(time.Time{})
	conn.NetConn().SetWriteDeadline(time.Time{})

	rtmpErr := make(chan error, 1)

	go func() {
		for {
			_, err := conn.Read()
			if err != nil {
				select {
				case rtmpErr <- fmt.Errorf("RTMP read error: %w", err):
				default:
				}
				return
			}
		}
	}()

	// Wait for error or context cancellation
	select {
	case err := <-reader.Error():
		return err
	case err := <-rtmpErr:
		return err
	case <-t.ctx.Done():
		return nil
	}
}

func (t *Target) runRTSP() error {
	t.Log(logger.Debug, "connecting to RTSP server")

	// Resolve the URL with path variables
	targetURL := t.resolveURL()

	u, err := base.ParseURL(targetURL)
	if err != nil {
		return err
	}

	t.mutex.RLock()
	strm := t.stream
	t.mutex.RUnlock()

	if strm == nil {
		return fmt.Errorf("stream is not available")
	}

	// Determine scheme
	scheme := "rtsp"
	if u.Scheme == "rtsps" {
		scheme = "rtsps"
	}

	// Create RTSP client for publishing
	client := &gortsplib.Client{
		Scheme:       scheme,
		Host:         u.Host,
		ReadTimeout:  time.Duration(t.ReadTimeout),
		WriteTimeout: time.Duration(t.WriteTimeout),
		TLSConfig:    mtls.MakeConfig(u.Host, ""),
	}

	err = client.Start()
	if err != nil {
		return err
	}
	defer client.Close()

	// Announce the stream
	_, err = client.Announce(u, strm.Desc)
	if err != nil {
		return err
	}

	// Setup all medias
	for _, media := range strm.Desc.Medias {
		_, err = client.Setup(u, media, 0, 0)
		if err != nil {
			return err
		}
	}

	// Start recording (publishing)
	_, err = client.Record()
	if err != nil {
		return err
	}

	t.Log(logger.Info, "connected to %s", targetURL)

	// Create reader
	reader := &stream.Reader{
		Parent: t,
	}

	// Setup data handlers for each media
	for _, media := range strm.Desc.Medias {
		for _, forma := range media.Formats {
			cmedia := media
			cforma := forma

			reader.OnData(
				cmedia,
				cforma,
				func(u *unit.Unit) error {
					if u.NilPayload() {
						return nil
					}

					// Write RTP packets to the client
					for _, pkt := range u.RTPPackets {
						err := client.WritePacketRTP(cmedia, pkt)
						if err != nil {
							return err
						}

						// Count bytes sent
						t.addBytesSent(uint64(pkt.MarshalSize()))
					}

					return nil
				})
		}
	}

	// Add reader to stream
	strm.AddReader(reader)
	defer strm.RemoveReader(reader)

	t.mutex.Lock()
	t.reader = reader
	t.mutex.Unlock()

	// Wait for error or context cancellation
	select {
	case err := <-reader.Error():
		return err
	case <-t.ctx.Done():
		return nil
	}
}

func (t *Target) runSRT() error {
	t.Log(logger.Debug, "connecting to SRT server")

	// Resolve the URL with path variables
	targetURL := t.resolveURL()

	conf := srt.DefaultConfig()
	address, err := conf.UnmarshalURL(targetURL)
	if err != nil {
		return err
	}

	err = conf.Validate()
	if err != nil {
		return err
	}

	t.mutex.RLock()
	strm := t.stream
	t.mutex.RUnlock()

	if strm == nil {
		return fmt.Errorf("stream is not available")
	}

	// Connect to SRT server
	sconn, err := srt.Dial("srt", address, conf)
	if err != nil {
		return err
	}
	defer sconn.Close()

	t.Log(logger.Info, "connected to %s", targetURL)

	// Create a counting writer to track bytes sent
	cw := &countingWriter{w: sconn, count: &t.bytesSent}
	bw := bufio.NewWriterSize(cw, 1316) // SRT max payload size

	// Create MPEG-TS writer
	var mpegtsWriter *mcmpegts.Writer
	var tracks []*mcmpegts.Track

	// Create reader
	reader := &stream.Reader{
		Parent: t,
	}

	// Setup tracks based on the stream description
	for _, media := range strm.Desc.Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.H265:
				track := &mcmpegts.Track{Codec: &tscodecs.H265{}}
				tracks = append(tracks, track)

				var dtsExtractor *h265.DTSExtractor

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if dtsExtractor == nil {
							if !h265.IsRandomAccess(u.Payload.(unit.PayloadH265)) {
								return nil
							}
							dtsExtractor = &h265.DTSExtractor{}
							dtsExtractor.Initialize()
						}

						dts, err := dtsExtractor.Extract(u.Payload.(unit.PayloadH265), u.PTS)
						if err != nil {
							return err
						}

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(t.WriteTimeout)))
						err = mpegtsWriter.WriteH265(track, u.PTS, dts, u.Payload.(unit.PayloadH265))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.H264:
				track := &mcmpegts.Track{Codec: &tscodecs.H264{}}
				tracks = append(tracks, track)

				var dtsExtractor *h264.DTSExtractor

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						idrPresent := h264.IsRandomAccess(u.Payload.(unit.PayloadH264))

						if dtsExtractor == nil {
							if !idrPresent {
								return nil
							}
							dtsExtractor = &h264.DTSExtractor{}
							dtsExtractor.Initialize()
						}

						dts, err := dtsExtractor.Extract(u.Payload.(unit.PayloadH264), u.PTS)
						if err != nil {
							return err
						}

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(t.WriteTimeout)))
						err = mpegtsWriter.WriteH264(track, u.PTS, dts, u.Payload.(unit.PayloadH264))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG4Audio:
				track := &mcmpegts.Track{Codec: &tscodecs.MPEG4Audio{
					Config: *forma.Config,
				}}
				tracks = append(tracks, track)

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(t.WriteTimeout)))
						err := mpegtsWriter.WriteMPEG4Audio(track, u.PTS, u.Payload.(unit.PayloadMPEG4Audio))
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.Opus:
				track := &mcmpegts.Track{Codec: &tscodecs.Opus{
					ChannelCount: forma.ChannelCount,
				}}
				tracks = append(tracks, track)

				reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(t.WriteTimeout)))
						err := mpegtsWriter.WriteOpus(track, u.PTS, u.Payload.(unit.PayloadOpus))
						if err != nil {
							return err
						}
						return bw.Flush()
					})
			}
		}
	}

	if len(tracks) == 0 {
		return fmt.Errorf("no supported tracks found for SRT push")
	}

	// Initialize MPEG-TS writer
	mpegtsWriter = &mcmpegts.Writer{W: bw, Tracks: tracks}
	err = mpegtsWriter.Initialize()
	if err != nil {
		return err
	}

	// Add reader to stream
	strm.AddReader(reader)
	defer strm.RemoveReader(reader)

	t.mutex.Lock()
	t.reader = reader
	t.mutex.Unlock()

	// Wait for error or context cancellation
	select {
	case err := <-reader.Error():
		return err
	case <-t.ctx.Done():
		return nil
	}
}

func (t *Target) resolveURL() string {
	result := t.URL
	result = strings.ReplaceAll(result, "$MTX_PATH", t.PathName)
	result = strings.ReplaceAll(result, "$path", t.PathName)
	return result
}
