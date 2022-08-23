package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpmpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/h264conf"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

const (
	rtmpConnPauseAfterAuthError = 2 * time.Second
)

func pathNameAndQuery(inURL *url.URL) (string, url.Values, string) {
	// remove leading and trailing slashes inserted by OBS and some other clients
	tmp := strings.TrimRight(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimLeft(ur.Path, "/")
	return pathName, ur.Query(), ur.RawQuery
}

type rtmpConnState int

const (
	rtmpConnStateIdle rtmpConnState = iota //nolint:deadcode,varcheck
	rtmpConnStateRead
	rtmpConnStatePublish
)

type rtmpConnPathManager interface {
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
	publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes
}

type rtmpConnParent interface {
	log(logger.Level, string, ...interface{})
	connClose(*rtmpConn)
}

type rtmpConn struct {
	isTLS                     bool
	id                        string
	externalAuthenticationURL string
	rtspAddress               string
	readTimeout               conf.StringDuration
	writeTimeout              conf.StringDuration
	readBufferCount           int
	runOnConnect              string
	runOnConnectRestart       bool
	wg                        *sync.WaitGroup
	conn                      *rtmp.Conn
	nconn                     net.Conn
	externalCmdPool           *externalcmd.Pool
	pathManager               rtmpConnPathManager
	parent                    rtmpConnParent

	ctx        context.Context
	ctxCancel  func()
	created    time.Time
	path       *path
	ringBuffer *ringbuffer.RingBuffer // read
	state      rtmpConnState
	stateMutex sync.Mutex
}

func newRTMPConn(
	parentCtx context.Context,
	isTLS bool,
	id string,
	externalAuthenticationURL string,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	runOnConnect string,
	runOnConnectRestart bool,
	wg *sync.WaitGroup,
	nconn net.Conn,
	externalCmdPool *externalcmd.Pool,
	pathManager rtmpConnPathManager,
	parent rtmpConnParent,
) *rtmpConn {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &rtmpConn{
		isTLS:                     isTLS,
		id:                        id,
		externalAuthenticationURL: externalAuthenticationURL,
		rtspAddress:               rtspAddress,
		readTimeout:               readTimeout,
		writeTimeout:              writeTimeout,
		readBufferCount:           readBufferCount,
		runOnConnect:              runOnConnect,
		runOnConnectRestart:       runOnConnectRestart,
		wg:                        wg,
		conn:                      rtmp.NewConn(nconn),
		nconn:                     nconn,
		externalCmdPool:           externalCmdPool,
		pathManager:               pathManager,
		parent:                    parent,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		created:                   time.Now(),
	}

	c.log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

func (c *rtmpConn) close() {
	c.ctxCancel()
}

func (c *rtmpConn) remoteAddr() net.Addr {
	return c.nconn.RemoteAddr()
}

func (c *rtmpConn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.log(level, "[conn %v] "+format, append([]interface{}{c.nconn.RemoteAddr()}, args...)...)
}

func (c *rtmpConn) ip() net.IP {
	return c.nconn.RemoteAddr().(*net.TCPAddr).IP
}

func (c *rtmpConn) safeState() rtmpConnState {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	return c.state
}

func (c *rtmpConn) run() {
	defer c.wg.Done()

	err := func() error {
		if c.runOnConnect != "" {
			c.log(logger.Info, "runOnConnect command started")
			_, port, _ := net.SplitHostPort(c.rtspAddress)
			onConnectCmd := externalcmd.NewCmd(
				c.externalCmdPool,
				c.runOnConnect,
				c.runOnConnectRestart,
				externalcmd.Environment{
					"RTSP_PATH": "",
					"RTSP_PORT": port,
				},
				func(co int) {
					c.log(logger.Info, "runOnConnect command exited with code %d", co)
				})

			defer func() {
				onConnectCmd.Close()
				c.log(logger.Info, "runOnConnect command stopped")
			}()
		}

		ctx, cancel := context.WithCancel(c.ctx)
		runErr := make(chan error)
		go func() {
			runErr <- c.runInner(ctx)
		}()

		select {
		case err := <-runErr:
			cancel()
			return err

		case <-c.ctx.Done():
			cancel()
			<-runErr
			return errors.New("terminated")
		}
	}()

	c.ctxCancel()

	c.parent.connClose(c)

	c.log(logger.Info, "closed (%v)", err)
}

func (c *rtmpConn) runInner(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		c.nconn.Close()
	}()

	c.nconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
	u, isPublishing, err := c.conn.InitializeServer()
	if err != nil {
		return err
	}

	if !isPublishing {
		return c.runRead(ctx, u)
	}
	return c.runPublish(ctx, u)
}

func (c *rtmpConn) runRead(ctx context.Context, u *url.URL) error {
	pathName, query, rawQuery := pathNameAndQuery(u)

	res := c.pathManager.readerAdd(pathReaderAddReq{
		author:   c,
		pathName: pathName,
		authenticate: func(
			pathIPs []fmt.Stringer,
			pathUser conf.Credential,
			pathPass conf.Credential,
		) error {
			return c.authenticate(pathName, pathIPs, pathUser, pathPass, false, query, rawQuery)
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(pathErrAuthCritical); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(rtmpConnPauseAfterAuthError)
			return errors.New(terr.message)
		}
		return res.err
	}

	c.path = res.path

	defer func() {
		c.path.readerRemove(pathReaderRemoveReq{author: c})
	}()

	c.stateMutex.Lock()
	c.state = rtmpConnStateRead
	c.stateMutex.Unlock()

	var videoTrack *gortsplib.TrackH264
	videoTrackID := -1
	var audioTrack *gortsplib.TrackMPEG4Audio
	audioTrackID := -1
	var aacDecoder *rtpmpeg4audio.Decoder

	for i, track := range res.stream.tracks() {
		switch tt := track.(type) {
		case *gortsplib.TrackH264:
			if videoTrack != nil {
				return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
			}

			videoTrack = tt
			videoTrackID = i

		case *gortsplib.TrackMPEG4Audio:
			if audioTrack != nil {
				return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
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

	c.ringBuffer, _ = ringbuffer.New(uint64(c.readBufferCount))
	go func() {
		<-ctx.Done()
		c.ringBuffer.Close()
	}()

	c.path.readerStart(pathReaderStartReq{
		author: c,
	})

	c.log(logger.Info, "is reading from path '%s', %s",
		c.path.Name(),
		sourceTrackInfo(res.stream.tracks()))

	if c.path.Conf().RunOnRead != "" {
		c.log(logger.Info, "runOnRead command started")
		onReadCmd := externalcmd.NewCmd(
			c.externalCmdPool,
			c.path.Conf().RunOnRead,
			c.path.Conf().RunOnReadRestart,
			c.path.externalCmdEnv(),
			func(co int) {
				c.log(logger.Info, "runOnRead command exited with code %d", co)
			})
		defer func() {
			onReadCmd.Close()
			c.log(logger.Info, "runOnRead command stopped")
		}()
	}

	err := c.conn.WriteTracks(videoTrack, audioTrack)
	if err != nil {
		return err
	}

	// disable read deadline
	c.nconn.SetReadDeadline(time.Time{})

	var videoInitialPTS *time.Duration
	videoFirstIDRFound := false
	var videoStartDTS time.Duration
	var videoDTSExtractor *h264.DTSExtractor

	for {
		item, ok := c.ringBuffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}
		data := item.(*data)

		if videoTrack != nil && data.trackID == videoTrackID {
			if data.h264NALUs == nil {
				continue
			}

			// video is decoded in another routine,
			// while audio is decoded in this routine:
			// we have to sync their PTS.
			if videoInitialPTS == nil {
				v := data.pts
				videoInitialPTS = &v
			}

			pts := data.pts - *videoInitialPTS

			idrPresent := false
			nonIDRPresent := false

			for _, nalu := range data.h264NALUs {
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeIDR:
					idrPresent = true

				case h264.NALUTypeNonIDR:
					nonIDRPresent = true
				}
			}

			var dts time.Duration

			// wait until we receive an IDR
			if !videoFirstIDRFound {
				if !idrPresent {
					continue
				}

				videoFirstIDRFound = true
				videoDTSExtractor = h264.NewDTSExtractor()

				var err error
				dts, err = videoDTSExtractor.Extract(data.h264NALUs, pts)
				if err != nil {
					return err
				}

				videoStartDTS = dts
				dts = 0
				pts -= videoStartDTS
			} else {
				if !idrPresent && !nonIDRPresent {
					continue
				}

				var err error
				dts, err = videoDTSExtractor.Extract(data.h264NALUs, pts)
				if err != nil {
					return err
				}

				dts -= videoStartDTS
				pts -= videoStartDTS
			}

			avcc, err := h264.AVCCMarshal(data.h264NALUs)
			if err != nil {
				return err
			}

			c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
			err = c.conn.WriteMessage(&message.MsgVideo{
				ChunkStreamID:   message.MsgVideoChunkStreamID,
				MessageStreamID: 0x1000000,
				IsKeyFrame:      idrPresent,
				H264Type:        flvio.AVC_NALU,
				Payload:         avcc,
				DTS:             dts,
				PTSDelta:        pts - dts,
			})
			if err != nil {
				return err
			}
		} else if audioTrack != nil && data.trackID == audioTrackID {
			aus, pts, err := aacDecoder.Decode(data.rtpPacket)
			if err != nil {
				if err != rtpmpeg4audio.ErrMorePacketsNeeded {
					c.log(logger.Warn, "unable to decode audio track: %v", err)
				}
				continue
			}

			if videoTrack != nil && !videoFirstIDRFound {
				continue
			}

			pts -= videoStartDTS
			if pts < 0 {
				continue
			}

			for i, au := range aus {
				c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
				err := c.conn.WriteMessage(&message.MsgAudio{
					ChunkStreamID:   message.MsgAudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         flvio.AAC_RAW,
					Payload:         au,
					DTS: pts + time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
						time.Second/time.Duration(audioTrack.ClockRate()),
				})
				if err != nil {
					return err
				}
			}
		}
	}
}

func (c *rtmpConn) runPublish(ctx context.Context, u *url.URL) error {
	pathName, query, rawQuery := pathNameAndQuery(u)

	res := c.pathManager.publisherAdd(pathPublisherAddReq{
		author:   c,
		pathName: pathName,
		authenticate: func(
			pathIPs []fmt.Stringer,
			pathUser conf.Credential,
			pathPass conf.Credential,
		) error {
			return c.authenticate(pathName, pathIPs, pathUser, pathPass, true, query, rawQuery)
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(pathErrAuthCritical); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(rtmpConnPauseAfterAuthError)
			return errors.New(terr.message)
		}
		return res.err
	}

	c.path = res.path

	defer func() {
		c.path.publisherRemove(pathPublisherRemoveReq{author: c})
	}()

	c.stateMutex.Lock()
	c.state = rtmpConnStatePublish
	c.stateMutex.Unlock()

	videoTrack, audioTrack, err := c.conn.ReadTracks()
	if err != nil {
		return err
	}

	var tracks gortsplib.Tracks
	videoTrackID := -1
	audioTrackID := -1

	if videoTrack != nil {
		videoTrackID = len(tracks)
		tracks = append(tracks, videoTrack)
	}

	if audioTrack != nil {
		audioTrackID = len(tracks)
		tracks = append(tracks, audioTrack)
	}

	rres := c.path.publisherStart(pathPublisherStartReq{
		author:             c,
		tracks:             tracks,
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	c.log(logger.Info, "is publishing to path '%s', %s",
		c.path.Name(),
		sourceTrackInfo(tracks))

	// disable write deadline to allow outgoing acknowledges
	c.nconn.SetWriteDeadline(time.Time{})

	for {
		c.nconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
		msg, err := c.conn.ReadMessage()
		if err != nil {
			return err
		}

		switch tmsg := msg.(type) {
		case *message.MsgVideo:
			if tmsg.H264Type == flvio.AVC_SEQHDR {
				var conf h264conf.Conf
				err = conf.Unmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to parse H264 config: %v", err)
				}

				nalus := [][]byte{
					conf.SPS,
					conf.PPS,
				}

				rres.stream.writeData(&data{
					trackID:      videoTrackID,
					ptsEqualsDTS: false,
					pts:          tmsg.DTS + tmsg.PTSDelta,
					h264NALUs:    nalus,
				})
			} else if tmsg.H264Type == flvio.AVC_NALU {
				if videoTrack == nil {
					return fmt.Errorf("received an H264 packet, but track is not set up")
				}

				nalus, err := h264.AVCCUnmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to decode AVCC: %v", err)
				}

				// skip invalid NALUs sent by DJI
				n := 0
				for _, nalu := range nalus {
					if len(nalu) != 0 {
						n++
					}
				}
				if n == 0 {
					continue
				}

				validNALUs := make([][]byte, n)
				pos := 0
				for _, nalu := range nalus {
					if len(nalu) != 0 {
						validNALUs[pos] = nalu
						pos++
					}
				}

				rres.stream.writeData(&data{
					trackID:      videoTrackID,
					ptsEqualsDTS: h264.IDRPresent(validNALUs),
					pts:          tmsg.DTS + tmsg.PTSDelta,
					h264NALUs:    validNALUs,
				})
			}

		case *message.MsgAudio:
			if tmsg.AACType == flvio.AAC_RAW {
				if audioTrack == nil {
					return fmt.Errorf("received an AAC packet, but track is not set up")
				}

				rres.stream.writeData(&data{
					trackID:      audioTrackID,
					ptsEqualsDTS: true,
					pts:          tmsg.DTS,
					mpeg4AudioAU: tmsg.Payload,
				})
			}
		}
	}
}

func (c *rtmpConn) authenticate(
	pathName string,
	pathIPs []fmt.Stringer,
	pathUser conf.Credential,
	pathPass conf.Credential,
	isPublishing bool,
	query url.Values,
	rawQuery string,
) error {
	if c.externalAuthenticationURL != "" {
		err := externalAuth(
			c.externalAuthenticationURL,
			c.ip().String(),
			query.Get("user"),
			query.Get("pass"),
			pathName,
			isPublishing,
			rawQuery)
		if err != nil {
			return pathErrAuthCritical{
				message: fmt.Sprintf("external authentication failed: %s", err),
			}
		}
	}

	if pathIPs != nil {
		ip := c.ip()
		if !ipEqualOrInRange(ip, pathIPs) {
			return pathErrAuthCritical{
				message: fmt.Sprintf("IP '%s' not allowed", ip),
			}
		}
	}

	if pathUser != "" {
		if query.Get("user") != string(pathUser) ||
			query.Get("pass") != string(pathPass) {
			return pathErrAuthCritical{
				message: "invalid credentials",
			}
		}
	}

	return nil
}

// onReaderData implements reader.
func (c *rtmpConn) onReaderData(data *data) {
	c.ringBuffer.Push(data)
}

// apiReaderDescribe implements reader.
func (c *rtmpConn) apiReaderDescribe() interface{} {
	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"rtmpConn", c.id}
}

// apiSourceDescribe implements source.
func (c *rtmpConn) apiSourceDescribe() interface{} {
	var typ string
	if c.isTLS {
		typ = "rtmpsConn"
	} else {
		typ = "rtmpConn"
	}

	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{typ, c.id}
}
