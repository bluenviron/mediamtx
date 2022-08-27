package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpmpeg4audio"
	"github.com/gin-gonic/gin"

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

<video id="video" muted controls autoplay playsinline></video>

<script src="https://cdn.jsdelivr.net/npm/hls.js@1.1.5"></script>

<script>

const create = () => {
	const video = document.getElementById('video');

	// always prefer hls.js over native HLS.
	// this is because some Android versions support native HLS
	// but don't support fMP4s.
	if (Hls.isSupported()) {
		const hls = new Hls({
			maxLiveSyncPlaybackRate: 1.5,
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

	} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
		// since it's not possible to detect timeout errors in iOS,
		// wait for the playlist to be available before starting the stream
		fetch('stream.m3u8')
			.then(() => {
				video.src = 'index.m3u8';
				video.play();
			});
	}
};

window.addEventListener('DOMContentLoaded', create);

</script>

</body>
</html>
`

type hlsMuxerRequest struct {
	dir  string
	file string
	ctx  *gin.Context
	res  chan func() *hls.MuxerFileResponse
}

type hlsMuxerPathManager interface {
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type hlsMuxerParent interface {
	log(logger.Level, string, ...interface{})
	muxerClose(*hlsMuxer)
}

type hlsMuxer struct {
	name                      string
	remoteAddr                string
	externalAuthenticationURL string
	hlsVariant                conf.HLSVariant
	hlsSegmentCount           int
	hlsSegmentDuration        conf.StringDuration
	hlsPartDuration           conf.StringDuration
	hlsSegmentMaxSize         conf.StringSize
	readBufferCount           int
	wg                        *sync.WaitGroup
	pathName                  string
	pathManager               hlsMuxerPathManager
	parent                    hlsMuxerParent

	ctx             context.Context
	ctxCancel       func()
	created         time.Time
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []*hlsMuxerRequest

	// in
	chRequest          chan *hlsMuxerRequest
	chAPIHLSMuxersList chan hlsServerAPIMuxersListSubReq
}

func newHLSMuxer(
	parentCtx context.Context,
	name string,
	remoteAddr string,
	externalAuthenticationURL string,
	hlsVariant conf.HLSVariant,
	hlsSegmentCount int,
	hlsSegmentDuration conf.StringDuration,
	hlsPartDuration conf.StringDuration,
	hlsSegmentMaxSize conf.StringSize,
	readBufferCount int,
	req *hlsMuxerRequest,
	wg *sync.WaitGroup,
	pathName string,
	pathManager hlsMuxerPathManager,
	parent hlsMuxerParent,
) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	m := &hlsMuxer{
		name:                      name,
		remoteAddr:                remoteAddr,
		externalAuthenticationURL: externalAuthenticationURL,
		hlsVariant:                hlsVariant,
		hlsSegmentCount:           hlsSegmentCount,
		hlsSegmentDuration:        hlsSegmentDuration,
		hlsPartDuration:           hlsPartDuration,
		hlsSegmentMaxSize:         hlsSegmentMaxSize,
		readBufferCount:           readBufferCount,
		wg:                        wg,
		pathName:                  pathName,
		pathManager:               pathManager,
		parent:                    parent,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		created:                   time.Now(),
		lastRequestTime: func() *int64 {
			v := time.Now().UnixNano()
			return &v
		}(),
		chRequest:          make(chan *hlsMuxerRequest),
		chAPIHLSMuxersList: make(chan hlsServerAPIMuxersListSubReq),
	}

	if req != nil {
		m.requests = append(m.requests, req)
	}

	m.log(logger.Info, "created %s", func() string {
		if remoteAddr == "" {
			return "automatically"
		}
		return "(requested by " + remoteAddr + ")"
	}())

	m.wg.Add(1)
	go m.run()

	return m
}

func (m *hlsMuxer) close() {
	m.ctxCancel()
}

func (m *hlsMuxer) log(level logger.Level, format string, args ...interface{}) {
	m.parent.log(level, "[muxer %s] "+format, append([]interface{}{m.pathName}, args...)...)
}

// PathName returns the path name.
func (m *hlsMuxer) PathName() string {
	return m.pathName
}

func (m *hlsMuxer) run() {
	defer m.wg.Done()

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	innerReady := make(chan struct{})
	innerErr := make(chan error)
	go func() {
		innerErr <- m.runInner(innerCtx, innerReady)
	}()

	isReady := false

	err := func() error {
		for {
			select {
			case <-m.ctx.Done():
				innerCtxCancel()
				<-innerErr
				return errors.New("terminated")

			case req := <-m.chRequest:
				if isReady {
					req.res <- m.handleRequest(req)
				} else {
					m.requests = append(m.requests, req)
				}

			case req := <-m.chAPIHLSMuxersList:
				req.data.Items[m.name] = hlsServerAPIMuxersListItem{
					Created:     m.created,
					LastRequest: time.Unix(0, atomic.LoadInt64(m.lastRequestTime)),
				}
				close(req.res)

			case <-innerReady:
				isReady = true
				for _, req := range m.requests {
					req.res <- m.handleRequest(req)
				}
				m.requests = nil

			case err := <-innerErr:
				innerCtxCancel()
				return err
			}
		}
	}()

	m.ctxCancel()

	for _, req := range m.requests {
		req.res <- func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{Status: http.StatusNotFound}
		}
	}

	m.parent.muxerClose(m)

	m.log(logger.Info, "destroyed (%v)", err)
}

func (m *hlsMuxer) runInner(innerCtx context.Context, innerReady chan struct{}) error {
	res := m.pathManager.readerAdd(pathReaderAddReq{
		author:       m,
		pathName:     m.pathName,
		authenticate: nil,
	})
	if res.err != nil {
		return res.err
	}

	m.path = res.path

	defer func() {
		m.path.readerRemove(pathReaderRemoveReq{author: m})
	}()

	var videoTrack *gortsplib.TrackH264
	videoTrackID := -1
	var audioTrack *gortsplib.TrackMPEG4Audio
	audioTrackID := -1
	var aacDecoder *rtpmpeg4audio.Decoder

	for i, track := range res.stream.tracks() {
		switch tt := track.(type) {
		case *gortsplib.TrackH264:
			if videoTrack != nil {
				return fmt.Errorf("can't encode track %d with HLS: too many tracks", i+1)
			}

			videoTrack = tt
			videoTrackID = i

		case *gortsplib.TrackMPEG4Audio:
			if audioTrack != nil {
				return fmt.Errorf("can't encode track %d with HLS: too many tracks", i+1)
			}

			audioTrack = tt
			audioTrackID = i
			aacDecoder = &rtpmpeg4audio.Decoder{
				SampleRate:       tt.Config.SampleRate,
				SizeLength:       tt.SizeLength,
				IndexLength:      tt.IndexLength,
				IndexDeltaLength: tt.IndexDeltaLength,
			}
			aacDecoder.Init()
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	var err error
	m.muxer, err = hls.NewMuxer(
		hls.MuxerVariant(m.hlsVariant),
		m.hlsSegmentCount,
		time.Duration(m.hlsSegmentDuration),
		time.Duration(m.hlsPartDuration),
		uint64(m.hlsSegmentMaxSize),
		videoTrack,
		audioTrack,
	)
	if err != nil {
		return fmt.Errorf("muxer error: %v", err)
	}
	defer m.muxer.Close()

	innerReady <- struct{}{}

	m.ringBuffer, _ = ringbuffer.New(uint64(m.readBufferCount))

	m.path.readerStart(pathReaderStartReq{author: m})

	m.log(logger.Info, "is converting into HLS, %s",
		sourceTrackInfo(res.stream.tracks()))

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
			var videoInitialPTS *time.Duration

			for {
				item, ok := m.ringBuffer.Pull()
				if !ok {
					return fmt.Errorf("terminated")
				}
				data := item.(*data)

				if videoTrack != nil && data.trackID == videoTrackID {
					if data.h264NALUs == nil {
						continue
					}

					if videoInitialPTS == nil {
						v := data.pts
						videoInitialPTS = &v
					}
					pts := data.pts - *videoInitialPTS

					err = m.muxer.WriteH264(time.Now(), pts, data.h264NALUs)
					if err != nil {
						return fmt.Errorf("muxer error: %v", err)
					}
				} else if audioTrack != nil && data.trackID == audioTrackID {
					aus, pts, err := aacDecoder.Decode(data.rtpPacket)
					if err != nil {
						if err != rtpmpeg4audio.ErrMorePacketsNeeded {
							m.log(logger.Warn, "unable to decode audio track: %v", err)
						}
						continue
					}

					for i, au := range aus {
						err = m.muxer.WriteAAC(
							time.Now(),
							pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
								time.Second/time.Duration(audioTrack.ClockRate()),
							au)
						if err != nil {
							return fmt.Errorf("muxer error: %v", err)
						}
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
			t := time.Unix(0, atomic.LoadInt64(m.lastRequestTime))
			if m.remoteAddr != "" && time.Since(t) >= closeAfterInactivity {
				m.ringBuffer.Close()
				<-writerDone
				return fmt.Errorf("not used anymore")
			}

		case err := <-writerDone:
			return err

		case <-innerCtx.Done():
			m.ringBuffer.Close()
			<-writerDone
			return fmt.Errorf("terminated")
		}
	}
}

func (m *hlsMuxer) handleRequest(req *hlsMuxerRequest) func() *hls.MuxerFileResponse {
	atomic.StoreInt64(m.lastRequestTime, time.Now().UnixNano())

	err := m.authenticate(req.ctx)
	if err != nil {
		if terr, ok := err.(pathErrAuthCritical); ok {
			m.log(logger.Info, "authentication error: %s", terr.message)
			return func() *hls.MuxerFileResponse {
				return &hls.MuxerFileResponse{
					Status: http.StatusUnauthorized,
				}
			}
		}

		return func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{
				Status: http.StatusUnauthorized,
				Header: map[string]string{
					"WWW-Authenticate": `Basic realm="rtsp-simple-server"`,
				},
			}
		}
	}

	if req.file == "" {
		return func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": `text/html`,
				},
				Body: bytes.NewReader([]byte(index)),
			}
		}
	}

	return func() *hls.MuxerFileResponse {
		return m.muxer.File(
			req.file,
			req.ctx.Query("_HLS_msn"),
			req.ctx.Query("_HLS_part"),
			req.ctx.Query("_HLS_skip"))
	}
}

func (m *hlsMuxer) authenticate(ctx *gin.Context) error {
	pathConf := m.path.Conf()
	pathIPs := pathConf.ReadIPs
	pathUser := pathConf.ReadUser
	pathPass := pathConf.ReadPass

	if m.externalAuthenticationURL != "" {
		ip := net.ParseIP(ctx.ClientIP())
		user, pass, ok := ctx.Request.BasicAuth()

		err := externalAuth(
			m.externalAuthenticationURL,
			ip.String(),
			user,
			pass,
			m.pathName,
			false,
			ctx.Request.URL.RawQuery)
		if err != nil {
			if !ok {
				return pathErrAuthNotCritical{}
			}

			return pathErrAuthCritical{
				message: fmt.Sprintf("external authentication failed: %s", err),
			}
		}
	}

	if pathIPs != nil {
		ip := net.ParseIP(ctx.ClientIP())

		if !ipEqualOrInRange(ip, pathIPs) {
			return pathErrAuthCritical{
				message: fmt.Sprintf("IP '%s' not allowed", ip),
			}
		}
	}

	if pathUser != "" {
		user, pass, ok := ctx.Request.BasicAuth()
		if !ok {
			return pathErrAuthNotCritical{}
		}

		if user != string(pathUser) || pass != string(pathPass) {
			return pathErrAuthCritical{
				message: "invalid credentials",
			}
		}
	}

	return nil
}

// request is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *hlsMuxer) request(req *hlsMuxerRequest) {
	select {
	case m.chRequest <- req:
	case <-m.ctx.Done():
		req.res <- func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{Status: http.StatusInternalServerError}
		}
	}
}

// apiHLSMuxersList is called by api.
func (m *hlsMuxer) apiHLSMuxersList(req hlsServerAPIMuxersListSubReq) {
	req.res = make(chan struct{})
	select {
	case m.chAPIHLSMuxersList <- req:
		<-req.res

	case <-m.ctx.Done():
	}
}

// onReaderData implements reader.
func (m *hlsMuxer) onReaderData(data *data) {
	m.ringBuffer.Push(data)
}

// apiReaderDescribe implements reader.
func (m *hlsMuxer) apiReaderDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsMuxer"}
}
