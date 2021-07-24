package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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

	ctx             context.Context
	ctxCancel       func()
	path            readPublisherPath
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer

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
		lastRequestTime: func() *int64 {
			v := time.Now().Unix()
			return &v
		}(),
		request: make(chan hlsConverterRequest),
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

	var err error
	c.muxer, err = hls.NewMuxer(
		c.hlsSegmentCount,
		c.hlsSegmentDuration,
		videoTrack,
		audioTrack,
	)
	if err != nil {
		return err
	}
	defer c.muxer.Close()

	// start request handler only after muxer has been inizialized
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
			var videoBuf [][]byte

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

					nalus, pts, err := h264Decoder.DecodeRTP(&pkt)
					if err != nil {
						if err != rtph264.ErrMorePacketsNeeded && err != rtph264.ErrNonStartingPacketAndNoPrevious {
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
					if pkt.Marker {
						err := c.muxer.WriteH264(pts, videoBuf)
						if err != nil {
							return err
						}

						videoBuf = nil
					}

				} else if audioTrack != nil && pair.trackID == audioTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						c.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					aus, pts, err := aacDecoder.DecodeRTP(&pkt)
					if err != nil {
						if err != rtpaac.ErrMorePacketsNeeded {
							c.log(logger.Warn, "unable to decode audio track: %v", err)
						}
						continue
					}

					err = c.muxer.WriteAAC(pts, aus)
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

func (c *hlsConverter) runRequestHandler(terminate chan struct{}, done chan struct{}) {
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
			case req.File == "stream.m3u8":
				r := c.muxer.Playlist()
				if r == nil {
					req.W.WriteHeader(http.StatusNotFound)
					req.Res <- nil
					continue
				}

				req.W.Header().Set("Content-Type", `application/x-mpegURL`)
				req.Res <- r

			case strings.HasSuffix(req.File, ".ts"):
				r := c.muxer.TSFile(req.File)
				if r == nil {
					req.W.WriteHeader(http.StatusNotFound)
					req.Res <- nil
					continue
				}

				req.W.Header().Set("Content-Type", `video/MP2T`)
				req.Res <- r

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
