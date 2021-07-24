package core

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
	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	// an offset is needed to
	// - avoid negative PTS values
	// - avoid PTS < DTS during startup
	hlsConverterPTSOffset = 2 * time.Second

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

type hlsConverterRequest struct {
	Dir  string
	File string
	Req  *http.Request
	W    http.ResponseWriter
	Res  chan io.Reader
}

type hlsConverterTrackIDPayloadPair struct {
	trackID int
	buf     []byte
}

type hlsConverterPathMan interface {
	OnReadPublisherSetupPlay(readPublisherSetupPlayReq)
}

type hlsConverterParent interface {
	Log(logger.Level, string, ...interface{})
	OnConverterClose(*hlsConverter)
}

type hlsConverter struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	readBufferCount    int
	wg                 *sync.WaitGroup
	stats              *stats
	pathName           string
	pathMan            hlsConverterPathMan
	parent             hlsConverterParent

	ctx                         context.Context
	ctxCancel                   func()
	path                        readPublisherPath
	ringBuffer                  *ringbuffer.RingBuffer
	tsQueue                     []*hls.TSFile
	tsByName                    map[string]*hls.TSFile
	tsDeleteCount               int
	tsMutex                     sync.RWMutex
	lasthlsConverterRequestTime *int64

	// in
	request chan hlsConverterRequest
}

func newHLSConverter(
	parentCtx context.Context,
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	readBufferCount int,
	wg *sync.WaitGroup,
	stats *stats,
	pathName string,
	pathMan hlsConverterPathMan,
	parent hlsConverterParent) *hlsConverter {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &hlsConverter{
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
		lasthlsConverterRequestTime: func() *int64 {
			v := time.Now().Unix()
			return &v
		}(),
		tsByName: make(map[string]*hls.TSFile),
		request:  make(chan hlsConverterRequest),
	}

	c.log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

// ParentClose closes a Converter.
func (c *hlsConverter) ParentClose() {
	c.log(logger.Info, "closed")
}

func (c *hlsConverter) Close() {
	c.ctxCancel()
}

// IsReadPublisher implements readPublisher.
func (c *hlsConverter) IsReadPublisher() {}

// IsSource implements source.
func (c *hlsConverter) IsSource() {}

func (c *hlsConverter) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[converter %s] "+format, append([]interface{}{c.pathName}, args...)...)
}

// PathName returns the path name of the readPublisher
func (c *hlsConverter) PathName() string {
	return c.pathName
}

func (c *hlsConverter) run() {
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
		c.path.OnReadPublisherRemove(readPublisherRemoveReq{c, res}) //nolint:govet
		<-res
	}

	c.parent.OnConverterClose(c)
}

func (c *hlsConverter) runInner(innerCtx context.Context) error {
	pres := make(chan readPublisherSetupPlayRes)
	c.pathMan.OnReadPublisherSetupPlay(readPublisherSetupPlayReq{
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

	curTSFile := hls.NewTSFile(videoTrack, audioTrack)
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

	resc := make(chan readPublisherPlayRes)
	c.path.OnReadPublisherPlay(readPublisherPlayReq{c, resc}) //nolint:govet
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
				pair := data.(hlsConverterTrackIDPayloadPair)

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
						if !bufferHasIDR && !curTSFile.FirstPacketWritten() {
							videoBuf = nil
							continue
						}

						err := func() error {
							c.tsMutex.Lock()
							defer c.tsMutex.Unlock()

							if bufferHasIDR {
								if curTSFile.FirstPacketWritten() &&
									curTSFile.Duration() >= c.hlsSegmentDuration {
									if curTSFile != nil {
										curTSFile.Close()
									}

									curTSFile = hls.NewTSFile(videoTrack, audioTrack)

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
								videoDTSEst.Feed(pts+hlsConverterPTSOffset),
								pts+hlsConverterPTSOffset,
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
							if curTSFile.FirstPacketWritten() &&
								curTSFile.Duration() >= c.hlsSegmentDuration &&
								audioAUCount >= segmentMinAUCount {

								if curTSFile != nil {
									curTSFile.Close()
								}

								audioAUCount = 0
								curTSFile = hls.NewTSFile(videoTrack, audioTrack)
								c.tsByName[curTSFile.Name()] = curTSFile
								c.tsQueue = append(c.tsQueue, curTSFile)
								if len(c.tsQueue) > c.hlsSegmentCount {
									delete(c.tsByName, c.tsQueue[0].Name())
									c.tsQueue = c.tsQueue[1:]
									c.tsDeleteCount++
								}
							}
						} else {
							if !curTSFile.FirstPacketWritten() {
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
								auPTS+hlsConverterPTSOffset,
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
			t := time.Unix(atomic.LoadInt64(c.lasthlsConverterRequestTime), 0)
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

func (c *hlsConverter) runRequestHandler(terminate chan struct{}, done chan struct{}) {
	defer close(done)

	for {
		select {
		case <-terminate:
			return

		case preq := <-c.request:
			req := preq

			atomic.StoreInt64(c.lasthlsConverterRequestTime, time.Now().Unix())

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
			case req.File == "stream.m3u8":
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

			case strings.HasSuffix(req.File, ".ts"):
				base := strings.TrimSuffix(req.File, ".ts")

				c.tsMutex.RLock()
				f, ok := c.tsByName[base]
				c.tsMutex.RUnlock()

				if !ok {
					req.W.WriteHeader(http.StatusNotFound)
					req.Res <- nil
					continue
				}

				req.W.Header().Set("Content-Type", `video/MP2T`)
				req.Res <- f.NewReader()

			case req.File == "":
				req.Res <- bytes.NewReader([]byte(index))

			default:
				req.W.WriteHeader(http.StatusNotFound)
				req.Res <- nil
			}
		}
	}
}

// OnRequest is called by hlsserver.Server.
func (c *hlsConverter) OnRequest(req hlsConverterRequest) {
	select {
	case c.request <- req:
	case <-c.ctx.Done():
		req.W.WriteHeader(http.StatusNotFound)
		req.Res <- nil
	}
}

// OnFrame implements path.Reader.
func (c *hlsConverter) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		c.ringBuffer.Push(hlsConverterTrackIDPayloadPair{trackID, payload})
	}
}
