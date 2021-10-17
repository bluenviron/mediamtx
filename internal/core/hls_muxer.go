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

	"github.com/aler9/rtsp-simple-server/internal/conf"
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
html, body {
	margin: 0;
	padding: 0;
	height: 100%;
}
#video {
	width: 100%;
	height: 100%;
	background: black;
}
</style>
</head>
<body>

<video id="video" muted controls autoplay></video>

<script src="https://cdn.jsdelivr.net/npm/hls.js@1.0.0"></script>

<script>

const create = () => {
	const video = document.getElementById('video');

	if (video.canPlayType('application/vnd.apple.mpegurl')) {
		// since it's not possible to detect timeout errors in iOS,
		// wait for the playlist to be available before starting the stream
		fetch('stream.m3u8')
			.then(() => {
				video.src = 'index.m3u8';
				video.play();
			});

	} else {
		const hls = new Hls({
			progressive: false,
		});

		hls.on(Hls.Events.ERROR, (evt, data) => {
			if (data.fatal) {
				hls.destroy();

				setTimeout(create, 2000);
			}
		});

		hls.loadSource('index.m3u8');
		hls.attachMedia(video);

		video.play();
	}
};

window.addEventListener('DOMContentLoaded', create);

</script>

</body>
</html>
`

type hlsMuxerResponse struct {
	Status int
	Header map[string]string
	Body   io.Reader
}

type hlsMuxerRequest struct {
	Dir  string
	File string
	Req  *http.Request
	Res  chan hlsMuxerResponse
}

type hlsMuxerTrackIDPayloadPair struct {
	trackID int
	buf     []byte
}

type hlsMuxerPathManager interface {
	OnReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
}

type hlsMuxerParent interface {
	Log(logger.Level, string, ...interface{})
	OnMuxerClose(*hlsMuxer)
}

type hlsMuxer struct {
	hlsAlwaysRemux     bool
	hlsSegmentCount    int
	hlsSegmentDuration conf.StringDuration
	readBufferCount    int
	wg                 *sync.WaitGroup
	pathName           string
	pathManager        hlsMuxerPathManager
	parent             hlsMuxerParent

	ctx             context.Context
	ctxCancel       func()
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []hlsMuxerRequest

	// in
	request chan hlsMuxerRequest
}

func newHLSMuxer(
	parentCtx context.Context,
	hlsAlwaysRemux bool,
	hlsSegmentCount int,
	hlsSegmentDuration conf.StringDuration,
	readBufferCount int,
	wg *sync.WaitGroup,
	pathName string,
	pathManager hlsMuxerPathManager,
	parent hlsMuxerParent) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	r := &hlsMuxer{
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
		request: make(chan hlsMuxerRequest),
	}

	r.log(logger.Info, "created")

	r.wg.Add(1)
	go r.run()

	return r
}

func (r *hlsMuxer) Close() {
	r.ctxCancel()
}

func (r *hlsMuxer) log(level logger.Level, format string, args ...interface{}) {
	r.parent.Log(level, "[muxer %s] "+format, append([]interface{}{r.pathName}, args...)...)
}

// PathName returns the path name.
func (r *hlsMuxer) PathName() string {
	return r.pathName
}

func (r *hlsMuxer) run() {
	defer r.wg.Done()
	defer r.log(logger.Info, "destroyed")

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	innerReady := make(chan struct{})
	innerErr := make(chan error)
	go func() {
		innerErr <- r.runInner(innerCtx, innerReady)
	}()

	isReady := false

outer:
	for {
		select {
		case <-r.ctx.Done():
			innerCtxCancel()
			<-innerErr
			break outer

		case req := <-r.request:
			if isReady {
				req.Res <- r.handleRequest(req)
			} else {
				r.requests = append(r.requests, req)
			}

		case <-innerReady:
			isReady = true
			for _, req := range r.requests {
				req.Res <- r.handleRequest(req)
			}
			r.requests = nil

		case err := <-innerErr:
			innerCtxCancel()
			if err != nil {
				r.log(logger.Info, "ERR: %s", err)
			}
			break outer
		}
	}

	r.ctxCancel()

	for _, req := range r.requests {
		req.Res <- hlsMuxerResponse{Status: http.StatusNotFound}
	}

	r.parent.OnMuxerClose(r)
}

func (r *hlsMuxer) runInner(innerCtx context.Context, innerReady chan struct{}) error {
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
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	audioTrackID := -1
	var aacDecoder *rtpaac.Decoder

	for i, t := range res.Stream.tracks() {
		if t.IsH264() {
			if videoTrack != nil {
				return fmt.Errorf("can't read track %d with HLS: too many tracks", i+1)
			}

			videoTrack = t
			videoTrackID = i

			h264Decoder = rtph264.NewDecoder()

		} else if t.IsAAC() {
			if audioTrack != nil {
				return fmt.Errorf("can't read track %d with HLS: too many tracks", i+1)
			}

			audioTrack = t
			audioTrackID = i

			conf, err := t.ExtractConfigAAC()
			if err != nil {
				return err
			}

			aacDecoder = rtpaac.NewDecoder(conf.SampleRate)
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	var err error
	r.muxer, err = hls.NewMuxer(
		r.hlsSegmentCount,
		time.Duration(r.hlsSegmentDuration),
		videoTrack,
		audioTrack,
	)
	if err != nil {
		return err
	}
	defer r.muxer.Close()

	innerReady <- struct{}{}

	r.ringBuffer = ringbuffer.New(uint64(r.readBufferCount))

	r.path.OnReaderPlay(pathReaderPlayReq{Author: r})

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
			for {
				data, ok := r.ringBuffer.Pull()
				if !ok {
					return fmt.Errorf("terminated")
				}
				pair := data.(hlsMuxerTrackIDPayloadPair)

				if videoTrack != nil && pair.trackID == videoTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						r.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					nalus, pts, err := h264Decoder.DecodeUntilMarker(&pkt)
					if err != nil {
						if err != rtph264.ErrMorePacketsNeeded &&
							err != rtph264.ErrNonStartingPacketAndNoPrevious {
							r.log(logger.Warn, "unable to decode video track: %v", err)
						}
						continue
					}

					err = r.muxer.WriteH264(pts, nalus)
					if err != nil {
						return err
					}

				} else if audioTrack != nil && pair.trackID == audioTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						r.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					aus, pts, err := aacDecoder.Decode(&pkt)
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

		case <-innerCtx.Done():
			r.ringBuffer.Close()
			<-writerDone
			return nil
		}
	}
}

func (r *hlsMuxer) handleRequest(req hlsMuxerRequest) hlsMuxerResponse {
	atomic.StoreInt64(r.lastRequestTime, time.Now().Unix())

	conf := r.path.Conf()

	if conf.ReadIPs != nil {
		tmp, _, _ := net.SplitHostPort(req.Req.RemoteAddr)
		ip := net.ParseIP(tmp)
		if !ipEqualOrInRange(ip, conf.ReadIPs) {
			r.log(logger.Info, "ERR: ip '%s' not allowed", ip)
			return hlsMuxerResponse{Status: http.StatusUnauthorized}
		}
	}

	if conf.ReadUser != "" {
		user, pass, ok := req.Req.BasicAuth()
		if !ok || user != string(conf.ReadUser) || pass != string(conf.ReadPass) {
			return hlsMuxerResponse{
				Status: http.StatusUnauthorized,
				Header: map[string]string{
					"WWW-Authenticate": `Basic realm="rtsp-simple-server"`,
				},
			}
		}
	}

	switch {
	case req.File == "index.m3u8":
		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `application/x-mpegURL`,
			},
			Body: r.muxer.PrimaryPlaylist(),
		}

	case req.File == "stream.m3u8":
		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `application/x-mpegURL`,
			},
			Body: r.muxer.StreamPlaylist(),
		}

	case strings.HasSuffix(req.File, ".ts"):
		r := r.muxer.Segment(req.File)
		if r == nil {
			return hlsMuxerResponse{Status: http.StatusNotFound}
		}

		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `video/MP2T`,
			},
			Body: r,
		}

	case req.File == "":
		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `text/html`,
			},
			Body: bytes.NewReader([]byte(index)),
		}

	default:
		return hlsMuxerResponse{Status: http.StatusNotFound}
	}
}

// OnRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (r *hlsMuxer) OnRequest(req hlsMuxerRequest) {
	select {
	case r.request <- req:
	case <-r.ctx.Done():
		req.Res <- hlsMuxerResponse{Status: http.StatusNotFound}
	}
}

// OnReaderAccepted implements reader.
func (r *hlsMuxer) OnReaderAccepted() {
	r.log(logger.Info, "is converting into HLS")
}

// OnReaderFrame implements reader.
func (r *hlsMuxer) OnReaderFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		r.ringBuffer.Push(hlsMuxerTrackIDPayloadPair{trackID, payload})
	}
}

// OnReaderAPIDescribe implements reader.
func (r *hlsMuxer) OnReaderAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsMuxer"}
}
