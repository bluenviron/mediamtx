package hlsconverter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/pion/rtp"

	"github.com/aler9/rtsp-simple-server/internal/h264"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/readpublisher"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	// an offset is needed to
	// - avoid negative PTS values
	// - avoid PTS < DTS during startup
	ptsOffset = 2 * time.Second

	segmentMinAUCount    = 100
	closeCheckPeriod     = 1 * time.Second
	closeAfterInactivity = 60 * time.Second
)

const index = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
#video {
	width: 600px;
	height: 600px;
	background: black;
}
</style>
</head>
<body>

<script src="https://cdn.jsdelivr.net/npm/hls.js@1.0.0"></script>
<video id="video" muted controls></video>
<script>

const create = () => {
	const video = document.getElementById('video');

	const hls = new Hls({
		progressive: false,
	});

	hls.on(Hls.Events.ERROR, (evt, data) => {
		if (data.fatal) {
			hls.destroy();

			setTimeout(() => {
				create();
			}, 2000);
		}
	});

	hls.loadSource('stream.m3u8');
	hls.attachMedia(video);

	video.play();
}
create();

</script>

</body>
</html>
`

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// Request is an HTTP request received by an HLS server.
type Request struct {
	Path     string
	FileName string
	Req      *http.Request
	W        http.ResponseWriter
	Res      chan io.Reader
}

type trackIDPayloadPair struct {
	trackID int
	buf     []byte
}

// PathMan is implemented by pathman.PathMan.
type PathMan interface {
	OnReadPublisherSetupPlay(readpublisher.SetupPlayReq)
}

// Parent is implemented by hlsserver.Server.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnConverterClose(*Converter)
}

// Converter is an HLS converter.
type Converter struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	readBufferCount    int
	wg                 *sync.WaitGroup
	stats              *stats.Stats
	pathName           string
	pathMan            PathMan
	parent             Parent

	ctx             context.Context
	ctxCancel       func()
	path            readpublisher.Path
	ringBuffer      *ringbuffer.RingBuffer
	tsQueue         []*tsFile
	tsByName        map[string]*tsFile
	tsDeleteCount   int
	tsMutex         sync.RWMutex
	lastRequestTime *int64

	// in
	request chan Request
}

// New allocates a Converter.
func New(
	ctxParent context.Context,
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	readBufferCount int,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	pathName string,
	pathMan PathMan,
	parent Parent) *Converter {
	ctx, ctxCancel := context.WithCancel(ctxParent)

	c := &Converter{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		readBufferCount:    readBufferCount,
		wg:                 wg,
		stats:              stats,
		pathName:           pathName,
		pathMan:            pathMan,
		parent:             parent,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		lastRequestTime: func() *int64 {
			v := time.Now().Unix()
			return &v
		}(),
		tsByName: make(map[string]*tsFile),
		request:  make(chan Request),
	}

	c.log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

// ParentClose closes a Converter.
func (c *Converter) ParentClose() {
	c.log(logger.Info, "closed")
}

// Close closes a Converter.
func (c *Converter) Close() {
	c.ctxCancel()
}

// IsReadPublisher implements readpublisher.ReadPublisher.
func (c *Converter) IsReadPublisher() {}

// IsSource implements source.Source.
func (c *Converter) IsSource() {}

func (c *Converter) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[converter %s] "+format, append([]interface{}{c.pathName}, args...)...)
}

// PathName returns the path name of the readpublisher.
func (c *Converter) PathName() string {
	return c.pathName
}

func (c *Converter) run() {
	defer c.wg.Done()

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	runErr := make(chan error)
	go func() {
		runErr <- c.runInner(innerCtx)
	}()

	select {
	case err := <-runErr:
		innerCtxCancel()
		if err != nil {
			c.log(logger.Info, "ERR: %s", err)
		}

	case <-c.ctx.Done():
		innerCtxCancel()
		<-runErr
	}

	c.ctxCancel()

	if c.path != nil {
		res := make(chan struct{})
		c.path.OnReadPublisherRemove(readpublisher.RemoveReq{c, res}) //nolint:govet
		<-res
	}

	c.parent.OnConverterClose(c)
}

func (c *Converter) runInner(innerCtx context.Context) error {
	pres := make(chan readpublisher.SetupPlayRes)
	c.pathMan.OnReadPublisherSetupPlay(readpublisher.SetupPlayReq{
		Author:              c,
		PathName:            c.pathName,
		IP:                  nil,
		ValidateCredentials: nil,
		Res:                 pres,
	})
	res := <-pres

	if res.Err != nil {
		return res.Err
	}

	c.path = res.Path
	var videoTrack *gortsplib.Track
	videoTrackID := -1
	var h264SPS []byte
	var h264PPS []byte
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	audioTrackID := -1
	var aacConfig rtpaac.MPEG4AudioConfig
	var aacDecoder *rtpaac.Decoder

	for i, t := range res.Stream.Tracks() {
		if t.IsH264() {
			if videoTrack != nil {
				return fmt.Errorf("can't read track %d with HLS: too many tracks", i+1)
			}

			videoTrack = t
			videoTrackID = i

			var err error
			h264SPS, h264PPS, err = t.ExtractDataH264()
			if err != nil {
				return err
			}

			h264Decoder = rtph264.NewDecoder()

		} else if t.IsAAC() {
			if audioTrack != nil {
				return fmt.Errorf("can't read track %d with HLS: too many tracks", i+1)
			}

			audioTrack = t
			audioTrackID = i

			byts, err := t.ExtractDataAAC()
			if err != nil {
				return err
			}

			err = aacConfig.Decode(byts)
			if err != nil {
				return err
			}

			aacDecoder = rtpaac.NewDecoder(aacConfig.SampleRate)
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return fmt.Errorf("unable to find a video or audio track")
	}

	curTSFile := newTSFile(videoTrack, audioTrack)
	c.tsByName[curTSFile.Name()] = curTSFile
	c.tsQueue = append(c.tsQueue, curTSFile)

	defer func() {
		curTSFile.Close()
	}()

	requestHandlerTerminate := make(chan struct{})
	requestHandlerDone := make(chan struct{})
	go c.runRequestHandler(requestHandlerTerminate, requestHandlerDone)

	defer func() {
		close(requestHandlerTerminate)
		<-requestHandlerDone
	}()

	c.ringBuffer = ringbuffer.New(uint64(c.readBufferCount))

	resc := make(chan readpublisher.PlayRes)
	c.path.OnReadPublisherPlay(readpublisher.PlayReq{c, resc}) //nolint:govet
	<-resc

	c.log(logger.Info, "is converting into HLS")

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
			startPCR := time.Now()
			var videoBuf [][]byte
			videoDTSEst := h264.NewDTSEstimator()
			videoInitialized := false
			audioAUCount := 0

			for {
				data, ok := c.ringBuffer.Pull()
				if !ok {
					return fmt.Errorf("terminated")
				}
				pair := data.(trackIDPayloadPair)

				if videoTrack != nil && pair.trackID == videoTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						c.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					// skip packets that are part of frames sent before
					// the initialization of the converter
					if !videoInitialized {
						typ := pkt.Payload[0] & 0x1F
						start := pkt.Payload[1] >> 7
						if typ == 28 && start != 1 { // FU-A
							continue
						}

						videoInitialized = true
					}

					nalus, pts, err := h264Decoder.DecodeRTP(&pkt)
					if err != nil {
						if err != rtph264.ErrMorePacketsNeeded {
							c.log(logger.Warn, "unable to decode video track: %v", err)
						}
						continue
					}

					for _, nalu := range nalus {
						// remove SPS, PPS, AUD
						typ := h264.NALUType(nalu[0] & 0x1F)
						switch typ {
						case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
							continue
						}

						// add SPS and PPS before IDR
						if typ == h264.NALUTypeIDR {
							videoBuf = append(videoBuf, h264SPS)
							videoBuf = append(videoBuf, h264PPS)
						}

						videoBuf = append(videoBuf, nalu)
					}

					// RTP marker means that all the NALUs with the same PTS have been received.
					// send them together.
					marker := (pair.buf[1] >> 7 & 0x1) > 0
					if marker {
						bufferHasIDR := func() bool {
							for _, nalu := range videoBuf {
								typ := h264.NALUType(nalu[0] & 0x1F)
								if typ == h264.NALUTypeIDR {
									return true
								}
							}
							return false
						}()

						// we received a marker packet but
						// - no IDR has been stored yet in current file
						// - there's no IDR in the buffer
						// data cannot be parsed, clear buffer
						if !bufferHasIDR && !curTSFile.firstPacketWritten {
							videoBuf = nil
							continue
						}

						err := func() error {
							c.tsMutex.Lock()
							defer c.tsMutex.Unlock()

							if bufferHasIDR {
								if curTSFile.firstPacketWritten &&
									curTSFile.Duration() >= c.hlsSegmentDuration {
									if curTSFile != nil {
										curTSFile.Close()
									}

									curTSFile = newTSFile(videoTrack, audioTrack)

									c.tsByName[curTSFile.Name()] = curTSFile
									c.tsQueue = append(c.tsQueue, curTSFile)
									if len(c.tsQueue) > c.hlsSegmentCount {
										delete(c.tsByName, c.tsQueue[0].Name())
										c.tsQueue = c.tsQueue[1:]
										c.tsDeleteCount++
									}
								}
							}

							curTSFile.SetPCR(time.Since(startPCR))
							err := curTSFile.WriteH264(
								videoDTSEst.Feed(pts+ptsOffset),
								pts+ptsOffset,
								bufferHasIDR,
								videoBuf)
							if err != nil {
								return err
							}

							videoBuf = nil
							return nil
						}()
						if err != nil {
							return err
						}
					}

				} else if audioTrack != nil && pair.trackID == audioTrackID {
					aus, pts, err := aacDecoder.Decode(pair.buf)
					if err != nil {
						if err != rtpaac.ErrMorePacketsNeeded {
							c.log(logger.Warn, "unable to decode audio track: %v", err)
						}
						continue
					}

					err = func() error {
						c.tsMutex.Lock()
						defer c.tsMutex.Unlock()

						if videoTrack == nil {
							if curTSFile.firstPacketWritten &&
								curTSFile.Duration() >= c.hlsSegmentDuration &&
								audioAUCount >= segmentMinAUCount {

								if curTSFile != nil {
									curTSFile.Close()
								}

								audioAUCount = 0
								curTSFile = newTSFile(videoTrack, audioTrack)
								c.tsByName[curTSFile.Name()] = curTSFile
								c.tsQueue = append(c.tsQueue, curTSFile)
								if len(c.tsQueue) > c.hlsSegmentCount {
									delete(c.tsByName, c.tsQueue[0].Name())
									c.tsQueue = c.tsQueue[1:]
									c.tsDeleteCount++
								}
							}
						} else {
							if !curTSFile.firstPacketWritten {
								return nil
							}
						}

						for i, au := range aus {
							auPTS := pts + time.Duration(i)*1000*time.Second/time.Duration(aacConfig.SampleRate)

							audioAUCount++
							curTSFile.SetPCR(time.Since(startPCR))
							err := curTSFile.WriteAAC(
								aacConfig.SampleRate,
								aacConfig.ChannelCount,
								auPTS+ptsOffset,
								au)
							if err != nil {
								return err
							}
						}

						return nil
					}()
					if err != nil {
						return err
					}
				}
			}
		}()
	}()

	closeCheckTicker := time.NewTicker(closeCheckPeriod)
	defer closeCheckTicker.Stop()

	for {
		select {
		case <-closeCheckTicker.C:
			t := time.Unix(atomic.LoadInt64(c.lastRequestTime), 0)
			if time.Since(t) >= closeAfterInactivity {
				c.ringBuffer.Close()
				<-writerDone
				return nil
			}

		case err := <-writerDone:
			return err

		case <-innerCtx.Done():
			c.ringBuffer.Close()
			<-writerDone
			return nil
		}
	}
}

func (c *Converter) runRequestHandler(terminate chan struct{}, done chan struct{}) {
	defer close(done)

	for {
		select {
		case <-terminate:
			return

		case preq := <-c.request:
			req := preq

			atomic.StoreInt64(c.lastRequestTime, time.Now().Unix())

			conf := c.path.Conf()

			if conf.ReadIPsParsed != nil {
				tmp, _, _ := net.SplitHostPort(req.Req.RemoteAddr)
				ip := net.ParseIP(tmp)
				if !ipEqualOrInRange(ip, conf.ReadIPsParsed) {
					c.log(logger.Info, "ERR: ip '%s' not allowed", ip)
					req.W.WriteHeader(http.StatusUnauthorized)
					req.Res <- nil
					continue
				}
			}

			if conf.ReadUser != "" {
				user, pass, ok := req.Req.BasicAuth()
				if !ok || user != conf.ReadUser || pass != conf.ReadPass {
					req.W.Header().Set("WWW-Authenticate", `Basic realm="rtsp-simple-server"`)
					req.W.WriteHeader(http.StatusUnauthorized)
					req.Res <- nil
					continue
				}
			}

			switch {
			case req.FileName == "stream.m3u8":
				func() {
					c.tsMutex.RLock()
					defer c.tsMutex.RUnlock()

					if len(c.tsQueue) == 0 {
						req.W.WriteHeader(http.StatusNotFound)
						req.Res <- nil
						return
					}

					cnt := "#EXTM3U\n"
					cnt += "#EXT-X-VERSION:3\n"
					cnt += "#EXT-X-ALLOW-CACHE:NO\n"

					targetDuration := func() uint {
						ret := uint(math.Ceil(c.hlsSegmentDuration.Seconds()))

						// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
						for _, f := range c.tsQueue {
							v2 := uint(math.Round(f.Duration().Seconds()))
							if v2 > ret {
								ret = v2
							}
						}

						return ret
					}()
					cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

					cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(c.tsDeleteCount), 10) + "\n"

					for _, f := range c.tsQueue {
						cnt += "#EXTINF:" + strconv.FormatFloat(f.Duration().Seconds(), 'f', -1, 64) + ",\n"
						cnt += f.Name() + ".ts\n"
					}

					req.W.Header().Set("Content-Type", `application/x-mpegURL`)
					req.Res <- bytes.NewReader([]byte(cnt))
				}()

			case strings.HasSuffix(req.FileName, ".ts"):
				base := strings.TrimSuffix(req.FileName, ".ts")

				c.tsMutex.RLock()
				f, ok := c.tsByName[base]
				c.tsMutex.RUnlock()

				if !ok {
					req.W.WriteHeader(http.StatusNotFound)
					req.Res <- nil
					continue
				}

				req.W.Header().Set("Content-Type", `video/MP2T`)
				req.Res <- f.buf.NewReader()

			case req.FileName == "":
				req.Res <- bytes.NewReader([]byte(index))

			default:
				req.W.WriteHeader(http.StatusNotFound)
				req.Res <- nil
			}
		}
	}
}

// OnRequest is called by hlsserver.Server.
func (c *Converter) OnRequest(req Request) {
	select {
	case c.request <- req:
	case <-c.ctx.Done():
		req.W.WriteHeader(http.StatusNotFound)
		req.Res <- nil
	}
}

// OnFrame implements path.Reader.
func (c *Converter) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		c.ringBuffer.Push(trackIDPayloadPair{trackID, payload})
	}
}
