// Package rpicamera contains the Raspberry Pi Camera static source.
package rpicamera

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmjpeg"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	pauseBetweenErrors = 1 * time.Second
)

func paramsFromConf(logLevel conf.LogLevel, cnf *conf.Path) params {
	return params{
		LogLevel: func() string {
			switch logLevel {
			case conf.LogLevel(logger.Debug):
				return "debug"
			case conf.LogLevel(logger.Info):
				return "info"
			case conf.LogLevel(logger.Warn):
				return "warn"
			}
			return "error"
		}(),
		CameraID:          uint32(cnf.RPICameraCamID),
		Width:             uint32(cnf.RPICameraWidth),
		Height:            uint32(cnf.RPICameraHeight),
		HFlip:             cnf.RPICameraHFlip,
		VFlip:             cnf.RPICameraVFlip,
		Brightness:        float32(cnf.RPICameraBrightness),
		Contrast:          float32(cnf.RPICameraContrast),
		Saturation:        float32(cnf.RPICameraSaturation),
		Sharpness:         float32(cnf.RPICameraSharpness),
		Exposure:          cnf.RPICameraExposure,
		AWB:               cnf.RPICameraAWB,
		AWBGainRed:        float32(cnf.RPICameraAWBGains[0]),
		AWBGainBlue:       float32(cnf.RPICameraAWBGains[1]),
		Denoise:           cnf.RPICameraDenoise,
		Shutter:           uint32(cnf.RPICameraShutter),
		Metering:          cnf.RPICameraMetering,
		Gain:              float32(cnf.RPICameraGain),
		EV:                float32(cnf.RPICameraEV),
		ROI:               cnf.RPICameraROI,
		HDR:               cnf.RPICameraHDR,
		TuningFile:        cnf.RPICameraTuningFile,
		Mode:              cnf.RPICameraMode,
		FPS:               float32(cnf.RPICameraFPS),
		AfMode:            cnf.RPICameraAfMode,
		AfRange:           cnf.RPICameraAfRange,
		AfSpeed:           cnf.RPICameraAfSpeed,
		LensPosition:      float32(cnf.RPICameraLensPosition),
		AfWindow:          cnf.RPICameraAfWindow,
		FlickerPeriod:     uint32(cnf.RPICameraFlickerPeriod),
		TextOverlayEnable: cnf.RPICameraTextOverlayEnable,
		TextOverlay:       cnf.RPICameraTextOverlay,
		Codec:             cnf.RPICameraCodec,
		IDRPeriod:         uint32(cnf.RPICameraIDRPeriod),
		Bitrate:           uint32(cnf.RPICameraBitrate),
		Profile:           cnf.RPICameraProfile,
		Level:             cnf.RPICameraLevel,
		SecondaryWidth:    uint32(cnf.RPICameraSecondaryWidth),
		SecondaryHeight:   uint32(cnf.RPICameraSecondaryHeight),
		SecondaryFPS:      float32(cnf.RPICameraSecondaryFPS),
		SecondaryQuality:  uint32(cnf.RPICameraSecondaryJPEGQuality),
	}
}

type secondaryReader struct {
	ctx       context.Context
	ctxCancel func()
}

// Close implements reader.
func (r *secondaryReader) Close() {
	r.ctxCancel()
}

// APIReaderDescribe implements reader.
func (*secondaryReader) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "rpiCameraSecondary",
		ID:   "",
	}
}

type parent interface {
	defs.StaticSourceParent
	AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error)
}

// Source is a Raspberry Pi Camera static source.
type Source struct {
	RTPMaxPayloadSize int
	LogLevel          conf.LogLevel
	Parent            parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[RPI Camera source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	if !params.Conf.RPICameraSecondary {
		return s.runPrimary(params)
	}
	return s.runSecondary(params)
}

func (s *Source) runPrimary(params defs.StaticSourceRunParams) error {
	var medias []*description.Media

	medi := &description.Media{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}},
	}
	medias = append(medias, medi)

	var mediaSecondary *description.Media

	if params.Conf.RPICameraSecondaryWidth != 0 {
		mediaSecondary = &description.Media{
			Type: description.MediaTypeApplication,
			Formats: []format.Format{&format.Generic{
				PayloadTyp: 96,
				RTPMa:      "rpicamera_secondary/90000",
				ClockRat:   90000,
			}},
		}
		medias = append(medias, mediaSecondary)
	}

	var strm *stream.Stream

	initializeStream := func() {
		if strm == nil {
			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:               &description.Session{Medias: medias},
				GenerateRTPPackets: false,
			})
			if res.Err != nil {
				panic("should not happen")
			}

			strm = res.Stream
		}
	}

	encH264 := &rtph264.Encoder{
		PayloadType:    96,
		PayloadMaxSize: s.RTPMaxPayloadSize,
	}
	err := encH264.Init()
	if err != nil {
		return err
	}

	onData := func(pts int64, ntp time.Time, au [][]byte) {
		initializeStream()

		pkts, err2 := encH264.Encode(au)
		if err2 != nil {
			s.Log(logger.Error, err2.Error())
			return
		}

		for _, pkt := range pkts {
			pkt.Timestamp = uint32(pts)
			strm.WriteRTPPacket(medi, medi.Formats[0], pkt, ntp, pts)
		}
	}

	var onDataSecondary func(pts int64, ntp time.Time, au []byte)

	if params.Conf.RPICameraSecondaryWidth != 0 {
		encJpeg := &rtpmjpeg.Encoder{
			PayloadMaxSize: s.RTPMaxPayloadSize,
		}
		err = encJpeg.Init()
		if err != nil {
			panic(err)
		}

		onDataSecondary = func(pts int64, ntp time.Time, au []byte) {
			initializeStream()

			pkts, err2 := encJpeg.Encode(au)
			if err2 != nil {
				s.Log(logger.Error, err2.Error())
				return
			}

			for _, pkt := range pkts {
				pkt.Timestamp = uint32(pts)
				pkt.PayloadType = 96
				strm.WriteRTPPacket(mediaSecondary, mediaSecondary.Formats[0], pkt, ntp, pts)
			}
		}
	}

	defer func() {
		if strm != nil {
			s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})
		}
	}()

	cam := &camera{
		params:          paramsFromConf(s.LogLevel, params.Conf),
		onData:          onData,
		onDataSecondary: onDataSecondary,
	}
	err = cam.initialize()
	if err != nil {
		return err
	}
	defer cam.close()

	cameraErr := make(chan error)
	go func() {
		cameraErr <- cam.wait()
	}()

	for {
		select {
		case err := <-cameraErr:
			return err

		case cnf := <-params.ReloadConf:
			cam.reloadParams(paramsFromConf(s.LogLevel, cnf))

		case <-params.Context.Done():
			return nil
		}
	}
}

func (s *Source) runSecondary(params defs.StaticSourceRunParams) error {
	r := &secondaryReader{}
	r.ctx, r.ctxCancel = context.WithCancel(context.Background())
	defer r.ctxCancel()

	path, origStream, err := s.waitForPrimary(r, params)
	if err != nil {
		return err
	}

	defer path.RemoveReader(defs.PathRemoveReaderReq{Author: r})

	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.MJPEG{}},
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: []*description.Media{media}},
		GenerateRTPPackets: false,
	})
	if res.Err != nil {
		return res.Err
	}

	origStream.AddReader(
		s,
		origStream.Desc.Medias[1],
		origStream.Desc.Medias[1].Formats[0],
		func(u unit.Unit) error {
			pkt := u.GetRTPPackets()[0]

			newPkt := &rtp.Packet{
				Header:  pkt.Header,
				Payload: pkt.Payload,
			}
			newPkt.PayloadType = 26

			res.Stream.WriteRTPPacket(media, media.Formats[0], newPkt, u.GetNTP(), u.GetPTS())
			return nil
		})

	origStream.StartReader(s)
	defer origStream.RemoveReader(s)

	select {
	case err := <-origStream.ReaderError(s):
		return err

	case <-r.ctx.Done():
		return fmt.Errorf("primary stream closed")

	case <-params.Context.Done():
		return fmt.Errorf("terminated")
	}
}

func (s *Source) waitForPrimary(
	r *secondaryReader,
	params defs.StaticSourceRunParams,
) (defs.Path, *stream.Stream, error) {
	for {
		path, origStream, err := s.Parent.AddReader(defs.PathAddReaderReq{
			Author: r,
			AccessRequest: defs.PathAccessRequest{
				Name:     params.Conf.RPICameraPrimaryName,
				SkipAuth: true,
			},
		})
		if err != nil {
			var err2 defs.PathNoStreamAvailableError
			if errors.As(err, &err2) {
				select {
				case <-time.After(pauseBetweenErrors):
				case <-params.Context.Done():
					return nil, nil, fmt.Errorf("terminated")
				}
				continue
			}

			return nil, nil, err
		}

		return path, origStream, nil
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "rpiCameraSource",
		ID:   "",
	}
}
