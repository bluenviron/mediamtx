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

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/ringbuffer"
	"github.com/google/uuid"
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

	ctx       context.Context
	ctxCancel func()
	uuid      uuid.UUID
	created   time.Time
	// path       *path
	state      rtmpConnState
	stateMutex sync.Mutex
}

func newRTMPConn(
	parentCtx context.Context,
	isTLS bool,
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
		uuid:                      uuid.New(),
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

	var err error
	select {
	case err = <-runErr:
		cancel()

	case <-c.ctx.Done():
		cancel()
		<-runErr
		err = errors.New("terminated")
	}

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

	path := res.path

	defer func() {
		path.readerRemove(pathReaderRemoveReq{author: c})
	}()

	c.stateMutex.Lock()
	c.state = rtmpConnStateRead
	c.stateMutex.Unlock()

	var videoFormat *format.H264
	videoMedia := res.stream.medias().FindFormat(&videoFormat)
	videoFirstIDRFound := false
	var videoStartDTS time.Duration

	var audioFormat *format.MPEG4Audio
	audioMedia := res.stream.medias().FindFormat(&audioFormat)

	if videoFormat == nil && audioFormat == nil {
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	ringBuffer, _ := ringbuffer.New(uint64(c.readBufferCount))
	go func() {
		<-ctx.Done()
		ringBuffer.Close()
	}()

	var medias media.Medias
	if videoMedia != nil {
		medias = append(medias, videoMedia)

		videoStartPTSFilled := false
		var videoStartPTS time.Duration
		var videoDTSExtractor *h264.DTSExtractor

		res.stream.readerAdd(c, videoMedia, videoFormat, func(dat data) {
			ringBuffer.Push(func() error {
				tdata := dat.(*dataH264)

				if tdata.au == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tdata.pts
				}
				pts := tdata.pts - videoStartPTS

				idrPresent := false
				nonIDRPresent := false

				for _, nalu := range tdata.au {
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
						return nil
					}

					videoFirstIDRFound = true
					videoDTSExtractor = h264.NewDTSExtractor()

					var err error
					dts, err = videoDTSExtractor.Extract(tdata.au, pts)
					if err != nil {
						return err
					}

					videoStartDTS = dts
					dts = 0
					pts -= videoStartDTS
				} else {
					if !idrPresent && !nonIDRPresent {
						return nil
					}

					var err error
					dts, err = videoDTSExtractor.Extract(tdata.au, pts)
					if err != nil {
						return err
					}

					dts -= videoStartDTS
					pts -= videoStartDTS
				}

				avcc, err := h264.AVCCMarshal(tdata.au)
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

				return nil
			})
		})
	}

	if audioMedia != nil {
		medias = append(medias, audioMedia)

		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		res.stream.readerAdd(c, audioMedia, audioFormat, func(dat data) {
			ringBuffer.Push(func() error {
				tdata := dat.(*dataMPEG4Audio)

				if tdata.aus == nil {
					return nil
				}

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tdata.pts
				}
				pts := tdata.pts - audioStartPTS

				if videoFormat != nil {
					if !videoFirstIDRFound {
						return nil
					}

					pts -= videoStartDTS
					if pts < 0 {
						return nil
					}
				}

				for i, au := range tdata.aus {
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
							time.Second/time.Duration(audioFormat.ClockRate()),
					})
					if err != nil {
						return err
					}
				}

				return nil
			})
		})
	}

	defer res.stream.readerRemove(c)

	c.log(logger.Info, "is reading from path '%s', %s",
		path.Name(), sourceMediaInfo(medias))

	if path.Conf().RunOnRead != "" {
		c.log(logger.Info, "runOnRead command started")
		onReadCmd := externalcmd.NewCmd(
			c.externalCmdPool,
			path.Conf().RunOnRead,
			path.Conf().RunOnReadRestart,
			path.externalCmdEnv(),
			func(co int) {
				c.log(logger.Info, "runOnRead command exited with code %d", co)
			})
		defer func() {
			onReadCmd.Close()
			c.log(logger.Info, "runOnRead command stopped")
		}()
	}

	err := c.conn.WriteTracks(videoFormat, audioFormat)
	if err != nil {
		return err
	}

	// disable read deadline
	c.nconn.SetReadDeadline(time.Time{})

	for {
		item, ok := ringBuffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := item.(func() error)()
		if err != nil {
			return err
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

	path := res.path

	defer func() {
		path.publisherRemove(pathPublisherRemoveReq{author: c})
	}()

	c.stateMutex.Lock()
	c.state = rtmpConnStatePublish
	c.stateMutex.Unlock()

	videoFormat, audioFormat, err := c.conn.ReadTracks()
	if err != nil {
		return err
	}

	var medias media.Medias
	var videoMedia *media.Media
	var audioMedia *media.Media

	if videoFormat != nil {
		videoMedia = &media.Media{
			Type:    media.TypeVideo,
			Formats: []format.Format{videoFormat},
		}
		medias = append(medias, videoMedia)
	}

	if audioFormat != nil {
		audioMedia = &media.Media{
			Type:    media.TypeAudio,
			Formats: []format.Format{audioFormat},
		}
		medias = append(medias, audioMedia)
	}

	rres := path.publisherStart(pathPublisherStartReq{
		author:             c,
		medias:             medias,
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	c.log(logger.Info, "is publishing to path '%s', %s",
		path.Name(),
		sourceMediaInfo(medias))

	// disable write deadline to allow outgoing acknowledges
	c.nconn.SetWriteDeadline(time.Time{})

	var onVideoData func(time.Duration, [][]byte)

	if _, ok := videoFormat.(*format.H264); ok {
		onVideoData = func(pts time.Duration, au [][]byte) {
			err = rres.stream.writeData(videoMedia, videoFormat, &dataH264{
				pts: pts,
				au:  au,
				ntp: time.Now(),
			})
			if err != nil {
				c.log(logger.Warn, "%v", err)
			}
		}
	} else {
		onVideoData = func(pts time.Duration, au [][]byte) {
			err = rres.stream.writeData(videoMedia, videoFormat, &dataH265{
				pts: pts,
				au:  au,
				ntp: time.Now(),
			})
			if err != nil {
				c.log(logger.Warn, "%v", err)
			}
		}
	}

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

				au := [][]byte{
					conf.SPS,
					conf.PPS,
				}

				err := rres.stream.writeData(videoMedia, videoFormat, &dataH264{
					pts: tmsg.DTS + tmsg.PTSDelta,
					au:  au,
					ntp: time.Now(),
				})
				if err != nil {
					c.log(logger.Warn, "%v", err)
				}
			} else if tmsg.H264Type == flvio.AVC_NALU {
				if videoFormat == nil {
					return fmt.Errorf("received a video packet, but track is not set up")
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

				onVideoData(tmsg.DTS+tmsg.PTSDelta, validNALUs)
			}

		case *message.MsgAudio:
			if tmsg.AACType == flvio.AAC_RAW {
				if audioFormat == nil {
					return fmt.Errorf("received an audio packet, but track is not set up")
				}

				err := rres.stream.writeData(audioMedia, audioFormat, &dataMPEG4Audio{
					pts: tmsg.DTS,
					aus: [][]byte{tmsg.Payload},
					ntp: time.Now(),
				})
				if err != nil {
					c.log(logger.Warn, "%v", err)
				}
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

// apiReaderDescribe implements reader.
func (c *rtmpConn) apiReaderDescribe() interface{} {
	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"rtmpConn", c.uuid.String()}
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
	}{typ, c.uuid.String()}
}
