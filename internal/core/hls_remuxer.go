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

type hlsRemuxerRequest struct {
	Dir  string
	File string
	Req  *http.Request
	W    http.ResponseWriter
	Res  chan io.Reader
}

type hlsRemuxerTrackIDPayloadPair struct {
	trackID int
	buf     []byte
}

type hlsRemuxerPathManager interface {
	OnReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
}

type hlsRemuxerParent interface {
	Log(logger.Level, string, ...interface{})
	OnRemuxerClose(*hlsRemuxer)
}

type hlsRemuxer struct {
	hlsAlwaysRemux     bool
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	readBufferCount    int
	wg                 *sync.WaitGroup
	pathName           string
	pathManager        hlsRemuxerPathManager
	parent             hlsRemuxerParent

	ctx             context.Context
	ctxCancel       func()
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []hlsRemuxerRequest

	// in
	request chan hlsRemuxerRequest
}

func newHLSRemuxer(
	parentCtx context.Context,
	hlsAlwaysRemux bool,
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	readBufferCount int,
	wg *sync.WaitGroup,
	pathName string,
	pathManager hlsRemuxerPathManager,
	parent hlsRemuxerParent) *hlsRemuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	r := &hlsRemuxer{
		hlsAlwaysRemux:     hlsAlwaysRemux,
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		readBufferCount:    readBufferCount,
		wg:                 wg,
		pathName:           pathName,
		pathManager:        pathManager,
		parent:             parent,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		lastRequestTime: func() *int64 {
			v := time.Now().Unix()
			return &v
		}(),
		request: make(chan hlsRemuxerRequest),
	}

	r.log(logger.Info, "created")

	r.wg.Add(1)
	go r.run()

	return r
}

func (r *hlsRemuxer) Close() {
	r.ctxCancel()
}

func (r *hlsRemuxer) log(level logger.Level, format string, args ...interface{}) {
	r.parent.Log(level, "[remuxer %s] "+format, append([]interface{}{r.pathName}, args...)...)
}

// PathName returns the path name.
func (r *hlsRemuxer) PathName() string {
	return r.pathName
}

func (r *hlsRemuxer) run() {
	defer r.wg.Done()
	defer r.log(logger.Info, "destroyed")

	remuxerCtx, remuxerCtxCancel := context.WithCancel(context.Background())
	remuxerReady := make(chan struct{})
	remuxerErr := make(chan error)
	go func() {
		remuxerErr <- r.runRemuxer(remuxerCtx, remuxerReady)
	}()

	isReady := false

outer:
	for {
		select {
		case <-r.ctx.Done():
			remuxerCtxCancel()
			<-remuxerErr
			break outer

		case req := <-r.request:
			if isReady {
				r.handleRequest(req)
			} else {
				r.requests = append(r.requests, req)
			}

		case <-remuxerReady:
			isReady = true
			for _, req := range r.requests {
				r.handleRequest(req)
			}
			r.requests = nil

		case err := <-remuxerErr:
			remuxerCtxCancel()
			if err != nil {
				r.log(logger.Info, "ERR: %s", err)
			}
			break outer
		}
	}

	r.ctxCancel()

	r.parent.OnRemuxerClose(r)
}

func (r *hlsRemuxer) runRemuxer(remuxerCtx context.Context, remuxerReady chan struct{}) error {
	res := r.pathManager.OnReaderSetupPlay(pathReaderSetupPlayReq{
		Author:              r,
		PathName:            r.pathName,
		IP:                  nil,
		ValidateCredentials: nil,
	})
	if res.Err != nil {
		return res.Err
	}

	r.path = res.Path

	defer func() {
		r.path.OnReaderRemove(pathReaderRemoveReq{Author: r})
	}()

	var videoTrack *gortsplib.Track
	videoTrackID := -1
	var h264SPS []byte
	var h264PPS []byte
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	audioTrackID := -1
	var aacConfig rtpaac.MPEG4AudioConfig
	var aacDecoder *rtpaac.Decoder

	for i, t := range res.Stream.tracks() {
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
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	var err error
	r.muxer, err = hls.NewMuxer(
		r.hlsSegmentCount,
		r.hlsSegmentDuration,
		videoTrack,
		audioTrack,
	)
	if err != nil {
		return err
	}
	defer r.muxer.Close()

	remuxerReady <- struct{}{}

	r.ringBuffer = ringbuffer.New(uint64(r.readBufferCount))

	r.path.OnReaderPlay(pathReaderPlayReq{Author: r})

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
			var videoBuf [][]byte

			for {
				data, ok := r.ringBuffer.Pull()
				if !ok {
					return fmt.Errorf("terminated")
				}
				pair := data.(hlsRemuxerTrackIDPayloadPair)

				if videoTrack != nil && pair.trackID == videoTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						r.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					nalus, pts, err := h264Decoder.DecodeRTP(&pkt)
					if err != nil {
						if err != rtph264.ErrMorePacketsNeeded && err != rtph264.ErrNonStartingPacketAndNoPrevious {
							r.log(logger.Warn, "unable to decode video track: %v", err)
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
						err := r.muxer.WriteH264(pts, videoBuf)
						if err != nil {
							return err
						}

						videoBuf = nil
					}

				} else if audioTrack != nil && pair.trackID == audioTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						r.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					aus, pts, err := aacDecoder.DecodeRTP(&pkt)
					if err != nil {
						if err != rtpaac.ErrMorePacketsNeeded {
							r.log(logger.Warn, "unable to decode audio track: %v", err)
						}
						continue
					}

					err = r.muxer.WriteAAC(pts, aus)
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
			t := time.Unix(atomic.LoadInt64(r.lastRequestTime), 0)
			if !r.hlsAlwaysRemux && time.Since(t) >= closeAfterInactivity {
				r.ringBuffer.Close()
				<-writerDone
				return nil
			}

		case err := <-writerDone:
			return err

		case <-remuxerCtx.Done():
			r.ringBuffer.Close()
			<-writerDone
			return nil
		}
	}
}

func (r *hlsRemuxer) handleRequest(req hlsRemuxerRequest) {
	atomic.StoreInt64(r.lastRequestTime, time.Now().Unix())

	conf := r.path.Conf()

	if conf.ReadIPsParsed != nil {
		tmp, _, _ := net.SplitHostPort(req.Req.RemoteAddr)
		ip := net.ParseIP(tmp)
		if !ipEqualOrInRange(ip, conf.ReadIPsParsed) {
			r.log(logger.Info, "ERR: ip '%s' not allowed", ip)
			req.W.WriteHeader(http.StatusUnauthorized)
			req.Res <- nil
			return
		}
	}

	if conf.ReadUser != "" {
		user, pass, ok := req.Req.BasicAuth()
		if !ok || user != conf.ReadUser || pass != conf.ReadPass {
			req.W.Header().Set("WWW-Authenticate", `Basic realm="rtsp-simple-server"`)
			req.W.WriteHeader(http.StatusUnauthorized)
			req.Res <- nil
			return
		}
	}

	switch {
	case req.File == "stream.m3u8":
		r := r.muxer.Playlist()
		if r == nil {
			req.W.WriteHeader(http.StatusNotFound)
			req.Res <- nil
			return
		}

		req.W.Header().Set("Content-Type", `application/x-mpegURL`)
		req.Res <- r

	case strings.HasSuffix(req.File, ".ts"):
		r := r.muxer.TSFile(req.File)
		if r == nil {
			req.W.WriteHeader(http.StatusNotFound)
			req.Res <- nil
			return
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

// OnRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (r *hlsRemuxer) OnRequest(req hlsRemuxerRequest) {
	select {
	case r.request <- req:
	case <-r.ctx.Done():
		req.W.WriteHeader(http.StatusNotFound)
		req.Res <- nil
	}
}

// OnReaderAccepted implements reader.
func (r *hlsRemuxer) OnReaderAccepted() {
	r.log(logger.Info, "is remuxing into HLS")
}

// OnReaderFrame implements reader.
func (r *hlsRemuxer) OnReaderFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		r.ringBuffer.Push(hlsRemuxerTrackIDPayloadPair{trackID, payload})
	}
}

// OnReaderAPIDescribe implements reader.
func (r *hlsRemuxer) OnReaderAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsremuxer"}
}
