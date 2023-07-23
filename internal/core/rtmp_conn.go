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

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg2audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/google/uuid"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/rtmp"
	"github.com/bluenviron/mediamtx/internal/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

const (
	rtmpPauseAfterAuthError = 2 * time.Second
)

func pathNameAndQuery(inURL *url.URL) (string, url.Values, string) {
	// remove leading and trailing slashes inserted by OBS and some other clients
	tmp := strings.TrimRight(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimLeft(ur.Path, "/")
	return pathName, ur.Query(), ur.RawQuery
}

type rtmpWriteFunc func(msg interface{}) error

func getRTMPWriteFunc(medi *media.Media, format formats.Format, stream *stream) rtmpWriteFunc {
	switch format.(type) {
	case *formats.H264:
		return func(msg interface{}) error {
			tmsg := msg.(*message.Video)

			switch tmsg.Type {
			case message.VideoTypeConfig:
				var conf h264conf.Conf
				err := conf.Unmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to parse H264 config: %v", err)
				}

				au := [][]byte{
					conf.SPS,
					conf.PPS,
				}

				stream.writeUnit(medi, format, &formatprocessor.UnitH264{
					PTS: tmsg.DTS + tmsg.PTSDelta,
					AU:  au,
					NTP: time.Now(),
				})

			case message.VideoTypeAU:
				au, err := h264.AVCCUnmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to decode AVCC: %v", err)
				}

				stream.writeUnit(medi, format, &formatprocessor.UnitH264{
					PTS: tmsg.DTS + tmsg.PTSDelta,
					AU:  au,
					NTP: time.Now(),
				})
			}

			return nil
		}

	case *formats.H265:
		return func(msg interface{}) error {
			switch tmsg := msg.(type) {
			case *message.Video:
				au, err := h264.AVCCUnmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to decode AVCC: %v", err)
				}

				stream.writeUnit(medi, format, &formatprocessor.UnitH265{
					PTS: tmsg.DTS + tmsg.PTSDelta,
					AU:  au,
					NTP: time.Now(),
				})

			case *message.ExtendedFramesX:
				au, err := h264.AVCCUnmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to decode AVCC: %v", err)
				}

				stream.writeUnit(medi, format, &formatprocessor.UnitH265{
					PTS: tmsg.DTS,
					AU:  au,
					NTP: time.Now(),
				})

			case *message.ExtendedCodedFrames:
				au, err := h264.AVCCUnmarshal(tmsg.Payload)
				if err != nil {
					return fmt.Errorf("unable to decode AVCC: %v", err)
				}

				stream.writeUnit(medi, format, &formatprocessor.UnitH265{
					PTS: tmsg.DTS + tmsg.PTSDelta,
					AU:  au,
					NTP: time.Now(),
				})
			}

			return nil
		}

	case *formats.AV1:
		return func(msg interface{}) error {
			if tmsg, ok := msg.(*message.ExtendedCodedFrames); ok {
				obus, err := av1.BitstreamUnmarshal(tmsg.Payload, true)
				if err != nil {
					return fmt.Errorf("unable to decode bitstream: %v", err)
				}

				stream.writeUnit(medi, format, &formatprocessor.UnitAV1{
					PTS:  tmsg.DTS,
					OBUs: obus,
					NTP:  time.Now(),
				})
			}

			return nil
		}

	case *formats.MPEG2Audio:
		return func(msg interface{}) error {
			tmsg := msg.(*message.Audio)

			stream.writeUnit(medi, format, &formatprocessor.UnitMPEG2Audio{
				PTS:    tmsg.DTS,
				Frames: [][]byte{tmsg.Payload},
				NTP:    time.Now(),
			})

			return nil
		}

	case *formats.MPEG4Audio:
		return func(msg interface{}) error {
			tmsg := msg.(*message.Audio)

			if tmsg.AACType == message.AudioAACTypeAU {
				stream.writeUnit(medi, format, &formatprocessor.UnitMPEG4AudioGeneric{
					PTS: tmsg.DTS,
					AUs: [][]byte{tmsg.Payload},
					NTP: time.Now(),
				})
			}

			return nil
		}
	}

	return nil
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
	logger.Writer
	connClose(*rtmpConn)
}

type rtmpConn struct {
	isTLS               bool
	rtspAddress         string
	readTimeout         conf.StringDuration
	writeTimeout        conf.StringDuration
	readBufferCount     int
	runOnConnect        string
	runOnConnectRestart bool
	wg                  *sync.WaitGroup
	conn                *rtmp.Conn
	nconn               net.Conn
	externalCmdPool     *externalcmd.Pool
	pathManager         rtmpConnPathManager
	parent              rtmpConnParent

	ctx       context.Context
	ctxCancel func()
	uuid      uuid.UUID
	created   time.Time
	mutex     sync.Mutex
	state     rtmpConnState
	pathName  string
}

func newRTMPConn(
	parentCtx context.Context,
	isTLS bool,
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
		isTLS:               isTLS,
		rtspAddress:         rtspAddress,
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		wg:                  wg,
		conn:                rtmp.NewConn(nconn),
		nconn:               nconn,
		externalCmdPool:     externalCmdPool,
		pathManager:         pathManager,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		uuid:                uuid.New(),
		created:             time.Now(),
	}

	c.Log(logger.Info, "opened")

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

func (c *rtmpConn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.nconn.RemoteAddr()}, args...)...)
}

func (c *rtmpConn) ip() net.IP {
	return c.nconn.RemoteAddr().(*net.TCPAddr).IP
}

func (c *rtmpConn) run() {
	defer c.wg.Done()

	if c.runOnConnect != "" {
		c.Log(logger.Info, "runOnConnect command started")
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		onConnectCmd := externalcmd.NewCmd(
			c.externalCmdPool,
			c.runOnConnect,
			c.runOnConnectRestart,
			externalcmd.Environment{
				"MTX_PATH":  "",
				"RTSP_PATH": "", // deprecated
				"RTSP_PORT": port,
			},
			func(err error) {
				c.Log(logger.Info, "runOnConnect command exited: %v", err)
			})

		defer func() {
			onConnectCmd.Close()
			c.Log(logger.Info, "runOnConnect command stopped")
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

	c.Log(logger.Info, "closed (%v)", err)
}

func (c *rtmpConn) runInner(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		c.nconn.Close()
	}()

	c.nconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
	u, publish, err := c.conn.InitializeServer()
	if err != nil {
		return err
	}

	if !publish {
		return c.runRead(ctx, u)
	}
	return c.runPublish(u)
}

func (c *rtmpConn) runRead(ctx context.Context, u *url.URL) error {
	pathName, query, rawQuery := pathNameAndQuery(u)

	res := c.pathManager.readerAdd(pathReaderAddReq{
		author:   c,
		pathName: pathName,
		credentials: authCredentials{
			query: rawQuery,
			ip:    c.ip(),
			user:  query.Get("user"),
			pass:  query.Get("pass"),
			proto: authProtocolRTMP,
			id:    &c.uuid,
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(rtmpPauseAfterAuthError)
			return terr
		}
		return res.err
	}

	defer res.path.readerRemove(pathReaderRemoveReq{author: c})

	c.mutex.Lock()
	c.state = rtmpConnStateRead
	c.pathName = pathName
	c.mutex.Unlock()

	ringBuffer, _ := ringbuffer.New(uint64(c.readBufferCount))
	go func() {
		<-ctx.Done()
		ringBuffer.Close()
	}()

	var medias media.Medias
	videoFirstIDRFound := false
	var videoStartDTS time.Duration

	videoMedia, videoFormat := c.findVideoFormat(res.stream, ringBuffer,
		&videoFirstIDRFound, &videoStartDTS)
	if videoMedia != nil {
		medias = append(medias, videoMedia)
	}

	audioMedia, audioFormat := c.findAudioFormat(res.stream, ringBuffer,
		videoFormat, &videoFirstIDRFound, &videoStartDTS)
	if audioFormat != nil {
		medias = append(medias, audioMedia)
	}

	if videoFormat == nil && audioFormat == nil {
		return fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently H264, MPEG-4 Audio, MPEG-1/2 Audio")
	}

	defer res.stream.readerRemove(c)

	c.Log(logger.Info, "is reading from path '%s', %s",
		res.path.name, sourceMediaInfo(medias))

	pathConf := res.path.safeConf()

	if pathConf.RunOnRead != "" {
		c.Log(logger.Info, "runOnRead command started")
		onReadCmd := externalcmd.NewCmd(
			c.externalCmdPool,
			pathConf.RunOnRead,
			pathConf.RunOnReadRestart,
			res.path.externalCmdEnv(),
			func(err error) {
				c.Log(logger.Info, "runOnRead command exited: %v", err)
			})
		defer func() {
			onReadCmd.Close()
			c.Log(logger.Info, "runOnRead command stopped")
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

func (c *rtmpConn) findVideoFormat(stream *stream, ringBuffer *ringbuffer.RingBuffer,
	videoFirstIDRFound *bool, videoStartDTS *time.Duration,
) (*media.Media, formats.Format) {
	var videoFormatH264 *formats.H264
	videoMedia := stream.medias().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		videoStartPTSFilled := false
		var videoStartPTS time.Duration
		var videoDTSExtractor *h264.DTSExtractor

		stream.readerAdd(c, videoMedia, videoFormatH264, func(unit formatprocessor.Unit) {
			ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitH264)

				if tunit.AU == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tunit.PTS
				}
				pts := tunit.PTS - videoStartPTS

				idrPresent := false
				nonIDRPresent := false

				for _, nalu := range tunit.AU {
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
				if !*videoFirstIDRFound {
					if !idrPresent {
						return nil
					}

					*videoFirstIDRFound = true
					videoDTSExtractor = h264.NewDTSExtractor()

					var err error
					dts, err = videoDTSExtractor.Extract(tunit.AU, pts)
					if err != nil {
						return err
					}

					*videoStartDTS = dts
					dts = 0
					pts -= *videoStartDTS
				} else {
					if !idrPresent && !nonIDRPresent {
						return nil
					}

					var err error
					dts, err = videoDTSExtractor.Extract(tunit.AU, pts)
					if err != nil {
						return err
					}

					dts -= *videoStartDTS
					pts -= *videoStartDTS
				}

				avcc, err := h264.AVCCMarshal(tunit.AU)
				if err != nil {
					return err
				}

				c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
				err = c.conn.WriteMessage(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      idrPresent,
					Type:            message.VideoTypeAU,
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

		return videoMedia, videoFormatH264
	}

	return nil, nil
}

func (c *rtmpConn) findAudioFormat(
	stream *stream,
	ringBuffer *ringbuffer.RingBuffer,
	videoFormat formats.Format,
	videoFirstIDRFound *bool,
	videoStartDTS *time.Duration,
) (*media.Media, formats.Format) {
	var audioFormatMPEG4Generic *formats.MPEG4AudioGeneric
	audioMedia := stream.medias().FindFormat(&audioFormatMPEG4Generic)

	if audioMedia != nil {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(c, audioMedia, audioFormatMPEG4Generic, func(unit formatprocessor.Unit) {
			ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitMPEG4AudioGeneric)

				if tunit.AUs == nil {
					return nil
				}

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tunit.PTS
				}
				pts := tunit.PTS - audioStartPTS

				if videoFormat != nil {
					if !*videoFirstIDRFound {
						return nil
					}

					pts -= *videoStartDTS
					if pts < 0 {
						return nil
					}
				}

				for i, au := range tunit.AUs {
					c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
					err := c.conn.WriteMessage(&message.Audio{
						ChunkStreamID:   message.AudioChunkStreamID,
						MessageStreamID: 0x1000000,
						Codec:           message.CodecMPEG4Audio,
						Rate:            flvio.SOUND_44Khz,
						Depth:           flvio.SOUND_16BIT,
						Channels:        flvio.SOUND_STEREO,
						AACType:         message.AudioAACTypeAU,
						Payload:         au,
						DTS: pts + time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
							time.Second/time.Duration(audioFormatMPEG4Generic.ClockRate()),
					})
					if err != nil {
						return err
					}
				}

				return nil
			})
		})

		return audioMedia, audioFormatMPEG4Generic
	}

	var audioFormatMPEG4AudioLATM *formats.MPEG4AudioLATM
	audioMedia = stream.medias().FindFormat(&audioFormatMPEG4AudioLATM)

	if audioMedia != nil &&
		audioFormatMPEG4AudioLATM.Config != nil &&
		len(audioFormatMPEG4AudioLATM.Config.Programs) == 1 &&
		len(audioFormatMPEG4AudioLATM.Config.Programs[0].Layers) == 1 {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(c, audioMedia, audioFormatMPEG4AudioLATM, func(unit formatprocessor.Unit) {
			ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitMPEG4AudioLATM)

				if tunit.AU == nil {
					return nil
				}

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tunit.PTS
				}
				pts := tunit.PTS - audioStartPTS

				if videoFormat != nil {
					if !*videoFirstIDRFound {
						return nil
					}

					pts -= *videoStartDTS
					if pts < 0 {
						return nil
					}
				}

				c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
				err := c.conn.WriteMessage(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeAU,
					Payload:         tunit.AU,
					DTS:             pts,
				})
				if err != nil {
					return err
				}

				return nil
			})
		})

		return audioMedia, audioFormatMPEG4AudioLATM
	}

	var audioFormatMPEG2 *formats.MPEG2Audio
	audioMedia = stream.medias().FindFormat(&audioFormatMPEG2)

	if audioMedia != nil {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(c, audioMedia, audioFormatMPEG2, func(unit formatprocessor.Unit) {
			ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitMPEG2Audio)

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tunit.PTS
				}
				pts := tunit.PTS - audioStartPTS

				if videoFormat != nil {
					if !*videoFirstIDRFound {
						return nil
					}

					pts -= *videoStartDTS
					if pts < 0 {
						return nil
					}
				}

				for _, frame := range tunit.Frames {
					var h mpeg2audio.FrameHeader
					err := h.Unmarshal(frame)
					if err != nil {
						return err
					}

					if !(!h.MPEG2 && h.Layer == 3) {
						return fmt.Errorf("RTMP only supports MPEG-1 layer 3 audio")
					}

					channels := uint8(flvio.SOUND_STEREO)
					if h.ChannelMode == mpeg2audio.ChannelModeMono {
						channels = flvio.SOUND_MONO
					}

					rate := uint8(flvio.SOUND_44Khz)
					switch h.SampleRate {
					case 5500:
						rate = flvio.SOUND_5_5Khz
					case 11025:
						rate = flvio.SOUND_11Khz
					case 22050:
						rate = flvio.SOUND_22Khz
					}

					msg := &message.Audio{
						ChunkStreamID:   message.AudioChunkStreamID,
						MessageStreamID: 0x1000000,
						Codec:           message.CodecMPEG2Audio,
						Rate:            rate,
						Depth:           flvio.SOUND_16BIT,
						Channels:        channels,
						Payload:         frame,
						DTS:             pts,
					}

					c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
					err = c.conn.WriteMessage(msg)
					if err != nil {
						return err
					}

					pts += time.Duration(h.SampleCount()) *
						time.Second / time.Duration(h.SampleRate)
				}

				return nil
			})
		})

		return audioMedia, audioFormatMPEG2
	}

	return nil, nil
}

func (c *rtmpConn) runPublish(u *url.URL) error {
	pathName, query, rawQuery := pathNameAndQuery(u)

	res := c.pathManager.publisherAdd(pathPublisherAddReq{
		author:   c,
		pathName: pathName,
		credentials: authCredentials{
			query: rawQuery,
			ip:    c.ip(),
			user:  query.Get("user"),
			pass:  query.Get("pass"),
			proto: authProtocolRTMP,
			id:    &c.uuid,
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(rtmpPauseAfterAuthError)
			return terr
		}
		return res.err
	}

	defer res.path.publisherRemove(pathPublisherRemoveReq{author: c})

	c.mutex.Lock()
	c.state = rtmpConnStatePublish
	c.pathName = pathName
	c.mutex.Unlock()

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
			Formats: []formats.Format{videoFormat},
		}
		medias = append(medias, videoMedia)
	}

	if audioFormat != nil {
		audioMedia = &media.Media{
			Type:    media.TypeAudio,
			Formats: []formats.Format{audioFormat},
		}
		medias = append(medias, audioMedia)
	}

	rres := res.path.publisherStart(pathPublisherStartReq{
		author:             c,
		medias:             medias,
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	c.Log(logger.Info, "is publishing to path '%s', %s",
		res.path.name,
		sourceMediaInfo(medias))

	// disable write deadline to allow outgoing acknowledges
	c.nconn.SetWriteDeadline(time.Time{})

	videoWriteFunc := getRTMPWriteFunc(videoMedia, videoFormat, rres.stream)
	audioWriteFunc := getRTMPWriteFunc(audioMedia, audioFormat, rres.stream)

	for {
		c.nconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
		msg, err := c.conn.ReadMessage()
		if err != nil {
			return err
		}

		switch msg.(type) {
		case *message.Video, *message.ExtendedFramesX, *message.ExtendedCodedFrames:
			if videoFormat == nil {
				return fmt.Errorf("received a video packet, but track is not set up")
			}

			err := videoWriteFunc(msg)
			if err != nil {
				c.Log(logger.Warn, "%v", err)
			}

		case *message.Audio:
			if audioFormat == nil {
				return fmt.Errorf("received an audio packet, but track is not set up")
			}

			err := audioWriteFunc(msg)
			if err != nil {
				c.Log(logger.Warn, "%v", err)
			}
		}
	}
}

// apiReaderDescribe implements reader.
func (c *rtmpConn) apiReaderDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: func() string {
			if c.isTLS {
				return "rtmpsConn"
			}
			return "rtmpConn"
		}(),
		ID: c.uuid.String(),
	}
}

// apiSourceDescribe implements source.
func (c *rtmpConn) apiSourceDescribe() pathAPISourceOrReader {
	return c.apiReaderDescribe()
}

func (c *rtmpConn) apiItem() *apiRTMPConn {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return &apiRTMPConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.remoteAddr().String(),
		State: func() string {
			switch c.state {
			case rtmpConnStateRead:
				return "read"

			case rtmpConnStatePublish:
				return "publish"
			}
			return "idle"
		}(),
		Path:          c.pathName,
		BytesReceived: c.conn.BytesReceived(),
		BytesSent:     c.conn.BytesSent(),
	}
}
