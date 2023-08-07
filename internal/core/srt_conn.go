package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func durationGoToMPEGTS(v time.Duration) int64 {
	return int64(v.Seconds() * 90000)
}

type srtConnState int

const (
	srtConnStateRead srtConnState = iota + 1
	srtConnStatePublish
)

type srtConnPathManager interface {
	addReader(req pathAddReaderReq) pathAddReaderRes
	addPublisher(req pathAddPublisherReq) pathAddPublisherRes
}

type srtConnParent interface {
	logger.Writer
	closeConn(*srtConn)
}

type srtConn struct {
	readTimeout       conf.StringDuration
	writeTimeout      conf.StringDuration
	readBufferCount   int
	udpMaxPayloadSize int
	connReq           srt.ConnRequest
	wg                *sync.WaitGroup
	externalCmdPool   *externalcmd.Pool
	pathManager       srtConnPathManager
	parent            srtConnParent

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	mutex     sync.RWMutex
	state     srtConnState
	pathName  string
	conn      srt.Conn

	chNew     chan srtNewConnReq
	chSetConn chan srt.Conn
}

func newSRTConn(
	parentCtx context.Context,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	udpMaxPayloadSize int,
	connReq srt.ConnRequest,
	wg *sync.WaitGroup,
	externalCmdPool *externalcmd.Pool,
	pathManager srtConnPathManager,
	parent srtConnParent,
) *srtConn {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &srtConn{
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		udpMaxPayloadSize: udpMaxPayloadSize,
		connReq:           connReq,
		wg:                wg,
		externalCmdPool:   externalCmdPool,
		pathManager:       pathManager,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		created:           time.Now(),
		uuid:              uuid.New(),
		chNew:             make(chan srtNewConnReq),
		chSetConn:         make(chan srt.Conn),
	}

	c.Log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

func (c *srtConn) close() {
	c.ctxCancel()
}

func (c *srtConn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.connReq.RemoteAddr()}, args...)...)
}

func (c *srtConn) ip() net.IP {
	return c.connReq.RemoteAddr().(*net.UDPAddr).IP
}

func (c *srtConn) run() {
	defer c.wg.Done()

	err := c.runInner()

	c.ctxCancel()

	c.parent.closeConn(c)

	c.Log(logger.Info, "closed (%v)", err)
}

func (c *srtConn) runInner() error {
	var req srtNewConnReq
	select {
	case req = <-c.chNew:
	case <-c.ctx.Done():
		return errors.New("terminated")
	}

	answerSent, err := c.runInner2(req)

	if !answerSent {
		req.res <- nil
	}

	return err
}

func (c *srtConn) runInner2(req srtNewConnReq) (bool, error) {
	parts := strings.Split(req.connReq.StreamId(), ":")
	if (len(parts) != 2 && len(parts) != 4) || (parts[0] != "read" && parts[0] != "publish") {
		return false, fmt.Errorf("invalid streamid '%s':"+
			" it must be 'action:pathname' or 'action:pathname:user:pass', "+
			"where action is either read or publish, pathname is the path name, user and pass are the credentials",
			req.connReq.StreamId())
	}

	pathName := parts[1]
	user := ""
	pass := ""

	if len(parts) == 4 {
		user, pass = parts[2], parts[3]
	}

	if parts[0] == "publish" {
		return c.runPublish(req, pathName, user, pass)
	}
	return c.runRead(req, pathName, user, pass)
}

func (c *srtConn) runPublish(req srtNewConnReq, pathName string, user string, pass string) (bool, error) {
	res := c.pathManager.addPublisher(pathAddPublisherReq{
		author:   c,
		pathName: pathName,
		credentials: authCredentials{
			ip:    c.ip(),
			user:  user,
			pass:  pass,
			proto: authProtocolSRT,
			id:    &c.uuid,
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			// TODO: re-enable. Currently this freezes the listener.
			// wait some seconds to stop brute force attacks
			// <-time.After(srtPauseAfterAuthError)
			return false, terr
		}
		return false, res.err
	}

	defer res.path.removePublisher(pathRemovePublisherReq{author: c})

	sconn, err := c.exchangeRequestWithConn(req)
	if err != nil {
		return true, err
	}

	c.mutex.Lock()
	c.state = srtConnStatePublish
	c.pathName = pathName
	c.conn = sconn
	c.mutex.Unlock()

	readerErr := make(chan error)
	go func() {
		readerErr <- c.runPublishReader(sconn, res.path)
	}()

	select {
	case err := <-readerErr:
		sconn.Close()
		return true, err

	case <-c.ctx.Done():
		sconn.Close()
		<-readerErr
		return true, errors.New("terminated")
	}
}

func (c *srtConn) runPublishReader(sconn srt.Conn, path *path) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	r, err := mpegts.NewReader(mpegts.NewBufferedReader(sconn))
	if err != nil {
		return err
	}

	var medias media.Medias
	var stream *stream.Stream

	var td *mpegts.TimeDecoder
	decodeTime := func(t int64) time.Duration {
		if td == nil {
			td = mpegts.NewTimeDecoder(t)
		}
		return td.Decode(t)
	}

	for _, track := range r.Tracks() { //nolint:dupl
		var medi *media.Media

		switch tcodec := track.Codec.(type) {
		case *mpegts.CodecH264:
			medi = &media.Media{
				Type: media.TypeVideo,
				Formats: []formats.Format{&formats.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitH264{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: decodeTime(pts),
					AU:  au,
				})
				return nil
			})

		case *mpegts.CodecH265:
			medi = &media.Media{
				Type: media.TypeVideo,
				Formats: []formats.Format{&formats.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitH265{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: decodeTime(pts),
					AU:  au,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Audio:
			medi = &media.Media{
				Type: media.TypeAudio,
				Formats: []formats.Format{&formats.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &tcodec.Config,
				}},
			}

			r.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitMPEG4AudioGeneric{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: decodeTime(pts),
					AUs: aus,
				})
				return nil
			})

		case *mpegts.CodecOpus:
			medi = &media.Media{
				Type: media.TypeAudio,
				Formats: []formats.Format{&formats.Opus{
					PayloadTyp: 96,
					IsStereo:   (tcodec.ChannelCount == 2),
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitOpus{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS:     decodeTime(pts),
					Packets: packets,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Audio:
			medi = &media.Media{
				Type:    media.TypeAudio,
				Formats: []formats.Format{&formats.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitMPEG1Audio{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS:    decodeTime(pts),
					Frames: frames,
				})
				return nil
			})

		default:
			continue
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	rres := path.startPublisher(pathStartPublisherReq{
		author:             c,
		medias:             medias,
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	stream = rres.stream

	for {
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

func (c *srtConn) runRead(req srtNewConnReq, pathName string, user string, pass string) (bool, error) {
	res := c.pathManager.addReader(pathAddReaderReq{
		author:   c,
		pathName: pathName,
		credentials: authCredentials{
			ip:    c.ip(),
			user:  user,
			pass:  pass,
			proto: authProtocolSRT,
			id:    &c.uuid,
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			// TODO: re-enable. Currently this freezes the listener.
			// wait some seconds to stop brute force attacks
			// <-time.After(srtPauseAfterAuthError)
			return false, terr
		}
		return false, res.err
	}

	defer res.path.removeReader(pathRemoveReaderReq{author: c})

	sconn, err := c.exchangeRequestWithConn(req)
	if err != nil {
		return true, err
	}
	defer sconn.Close()

	c.mutex.Lock()
	c.state = srtConnStateRead
	c.pathName = pathName
	c.conn = sconn
	c.mutex.Unlock()

	ringBuffer, _ := ringbuffer.New(uint64(c.readBufferCount))
	go func() {
		<-c.ctx.Done()
		ringBuffer.Close()
	}()

	var w *mpegts.Writer
	var tracks []*mpegts.Track
	var medias media.Medias
	bw := bufio.NewWriterSize(sconn, srtMaxPayloadSize(c.udpMaxPayloadSize))

	leadingTrackChosen := false
	leadingTrackInitialized := false
	var leadingTrackStartDTS time.Duration

	addTrack := func(medi *media.Media, codec mpegts.Codec) *mpegts.Track {
		track := &mpegts.Track{
			Codec: codec,
		}
		tracks = append(tracks, track)
		medias = append(medias, medi)
		return track
	}

	for _, medi := range res.stream.Medias() {
		for _, format := range medi.Formats {
			switch format := format.(type) {
			case *formats.H265: //nolint:dupl
				track := addTrack(medi, &mpegts.CodecH265{})

				var startPTS time.Duration
				startPTSFilled := false

				var isLeadingTrack bool
				if !leadingTrackChosen {
					isLeadingTrack = true
				} else {
					isLeadingTrack = false
				}

				randomAccessReceived := false
				dtsExtractor := h265.NewDTSExtractor()

				res.stream.AddReader(c, medi, format, func(unit formatprocessor.Unit) {
					ringBuffer.Push(func() error {
						tunit := unit.(*formatprocessor.UnitH265)
						if tunit.AU == nil {
							return nil
						}

						if !startPTSFilled {
							startPTS = tunit.PTS
							startPTSFilled = true
						}

						randomAccess := h265.IsRandomAccess(tunit.AU)

						if !randomAccessReceived {
							if !randomAccess {
								return nil
							}
							randomAccessReceived = true
						}

						pts := tunit.PTS - startPTS
						dts, err := dtsExtractor.Extract(tunit.AU, pts)
						if err != nil {
							return err
						}

						if !leadingTrackInitialized {
							if isLeadingTrack {
								leadingTrackStartDTS = dts
								leadingTrackInitialized = true
							} else {
								return nil
							}
						}

						dts -= leadingTrackStartDTS
						pts -= leadingTrackStartDTS

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
						err = w.WriteH26x(track, durationGoToMPEGTS(pts), durationGoToMPEGTS(dts), randomAccess, tunit.AU)
						if err != nil {
							return err
						}
						return bw.Flush()
					})
				})

			case *formats.H264: //nolint:dupl
				track := addTrack(medi, &mpegts.CodecH264{})

				var startPTS time.Duration
				startPTSFilled := false

				var isLeadingTrack bool
				if !leadingTrackChosen {
					isLeadingTrack = true
				} else {
					isLeadingTrack = false
				}

				firstIDRReceived := false
				dtsExtractor := h264.NewDTSExtractor()

				res.stream.AddReader(c, medi, format, func(unit formatprocessor.Unit) {
					ringBuffer.Push(func() error {
						tunit := unit.(*formatprocessor.UnitH264)
						if tunit.AU == nil {
							return nil
						}

						if !startPTSFilled {
							startPTS = tunit.PTS
							startPTSFilled = true
						}

						idrPresent := h264.IDRPresent(tunit.AU)

						if !firstIDRReceived {
							if !idrPresent {
								return nil
							}
							firstIDRReceived = true
						}

						pts := tunit.PTS - startPTS
						dts, err := dtsExtractor.Extract(tunit.AU, pts)
						if err != nil {
							return err
						}

						if !leadingTrackInitialized {
							if isLeadingTrack {
								leadingTrackStartDTS = dts
								leadingTrackInitialized = true
							} else {
								return nil
							}
						}

						dts -= leadingTrackStartDTS
						pts -= leadingTrackStartDTS

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
						err = w.WriteH26x(track, durationGoToMPEGTS(pts), durationGoToMPEGTS(dts), idrPresent, tunit.AU)
						if err != nil {
							return err
						}
						return bw.Flush()
					})
				})

			case *formats.MPEG4AudioGeneric:
				track := addTrack(medi, &mpegts.CodecMPEG4Audio{
					Config: *format.Config,
				})

				var startPTS time.Duration
				startPTSFilled := false

				res.stream.AddReader(c, medi, format, func(unit formatprocessor.Unit) {
					ringBuffer.Push(func() error {
						tunit := unit.(*formatprocessor.UnitMPEG4AudioGeneric)
						if tunit.AUs == nil {
							return nil
						}

						if !startPTSFilled {
							startPTS = tunit.PTS
							startPTSFilled = true
						}

						if leadingTrackChosen && !leadingTrackInitialized {
							return nil
						}

						pts := tunit.PTS
						pts -= startPTS
						pts -= leadingTrackStartDTS

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
						err = w.WriteMPEG4Audio(track, durationGoToMPEGTS(pts), tunit.AUs)
						if err != nil {
							return err
						}
						return bw.Flush()
					})
				})

			case *formats.MPEG4AudioLATM:
				if format.Config != nil &&
					len(format.Config.Programs) == 1 &&
					len(format.Config.Programs[0].Layers) == 1 {
					track := addTrack(medi, &mpegts.CodecMPEG4Audio{
						Config: *format.Config.Programs[0].Layers[0].AudioSpecificConfig,
					})

					var startPTS time.Duration
					startPTSFilled := false

					res.stream.AddReader(c, medi, format, func(unit formatprocessor.Unit) {
						ringBuffer.Push(func() error {
							tunit := unit.(*formatprocessor.UnitMPEG4AudioLATM)
							if tunit.AU == nil {
								return nil
							}

							if !startPTSFilled {
								startPTS = tunit.PTS
								startPTSFilled = true
							}

							if leadingTrackChosen && !leadingTrackInitialized {
								return nil
							}

							pts := tunit.PTS
							pts -= startPTS
							pts -= leadingTrackStartDTS

							sconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
							err = w.WriteMPEG4Audio(track, durationGoToMPEGTS(pts), [][]byte{tunit.AU})
							if err != nil {
								return err
							}
							return bw.Flush()
						})
					})
				}

			case *formats.Opus:
				track := addTrack(medi, &mpegts.CodecOpus{
					ChannelCount: func() int {
						if format.IsStereo {
							return 2
						}
						return 1
					}(),
				})

				var startPTS time.Duration
				startPTSFilled := false

				res.stream.AddReader(c, medi, format, func(unit formatprocessor.Unit) {
					ringBuffer.Push(func() error {
						tunit := unit.(*formatprocessor.UnitOpus)
						if tunit.Packets == nil {
							return nil
						}

						if !startPTSFilled {
							startPTS = tunit.PTS
							startPTSFilled = true
						}

						if leadingTrackChosen && !leadingTrackInitialized {
							return nil
						}

						pts := tunit.PTS
						pts -= startPTS
						pts -= leadingTrackStartDTS

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
						err = w.WriteOpus(track, durationGoToMPEGTS(pts), tunit.Packets)
						if err != nil {
							return err
						}
						return bw.Flush()
					})
				})

			case *formats.MPEG1Audio:
				track := addTrack(medi, &mpegts.CodecMPEG1Audio{})

				var startPTS time.Duration
				startPTSFilled := false

				res.stream.AddReader(c, medi, format, func(unit formatprocessor.Unit) {
					ringBuffer.Push(func() error {
						tunit := unit.(*formatprocessor.UnitMPEG1Audio)
						if tunit.Frames == nil {
							return nil
						}

						if !startPTSFilled {
							startPTS = tunit.PTS
							startPTSFilled = true
						}

						if leadingTrackChosen && !leadingTrackInitialized {
							return nil
						}

						pts := tunit.PTS
						pts -= startPTS
						pts -= leadingTrackStartDTS

						sconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
						err = w.WriteMPEG1Audio(track, durationGoToMPEGTS(pts), tunit.Frames)
						if err != nil {
							return err
						}
						return bw.Flush()
					})
				})
			}
		}
	}

	if len(tracks) == 0 {
		return true, fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently H265, H264, Opus, MPEG-4 Audio")
	}

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

	w = mpegts.NewWriter(bw, tracks)

	// disable read deadline
	sconn.SetReadDeadline(time.Time{})

	for {
		item, ok := ringBuffer.Pull()
		if !ok {
			return true, fmt.Errorf("terminated")
		}

		err := item.(func() error)()
		if err != nil {
			return true, err
		}
	}
}

func (c *srtConn) exchangeRequestWithConn(req srtNewConnReq) (srt.Conn, error) {
	req.res <- c

	select {
	case sconn := <-c.chSetConn:
		return sconn, nil

	case <-c.ctx.Done():
		return nil, errors.New("terminated")
	}
}

// new is called by srtListener through srtServer.
func (c *srtConn) new(req srtNewConnReq) *srtConn {
	select {
	case c.chNew <- req:
		return <-req.res

	case <-c.ctx.Done():
		return nil
	}
}

// setConn is called by srtListener .
func (c *srtConn) setConn(sconn srt.Conn) {
	select {
	case c.chSetConn <- sconn:
	case <-c.ctx.Done():
	}
}

// apiReaderDescribe implements reader.
func (c *srtConn) apiReaderDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "srtConn",
		ID:   c.uuid.String(),
	}
}

// apiSourceDescribe implements source.
func (c *srtConn) apiSourceDescribe() pathAPISourceOrReader {
	return c.apiReaderDescribe()
}

func (c *srtConn) apiItem() *apiSRTConn {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	bytesReceived := uint64(0)
	bytesSent := uint64(0)

	if c.conn != nil {
		var s srt.Statistics
		c.conn.Stats(&s)
		bytesReceived = s.Accumulated.ByteRecv
		bytesSent = s.Accumulated.ByteSent
	}

	return &apiSRTConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.connReq.RemoteAddr().String(),
		State: func() apiSRTConnState {
			switch c.state {
			case srtConnStateRead:
				return apiSRTConnStateRead

			case srtConnStatePublish:
				return apiSRTConnStatePublish

			default:
				return apiSRTConnStateIdle
			}
		}(),
		Path:          c.pathName,
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
