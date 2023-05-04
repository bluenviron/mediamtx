package core

import (
	"context"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/formatprocessor"
	"github.com/aler9/mediamtx/internal/logger"
	"github.com/aler9/mediamtx/internal/rpicamera"
)

func paramsFromConf(cnf *conf.PathConf) rpicamera.Params {
	return rpicamera.Params{
		CameraID:          cnf.RPICameraCamID,
		Width:             cnf.RPICameraWidth,
		Height:            cnf.RPICameraHeight,
		HFlip:             cnf.RPICameraHFlip,
		VFlip:             cnf.RPICameraVFlip,
		Brightness:        cnf.RPICameraBrightness,
		Contrast:          cnf.RPICameraContrast,
		Saturation:        cnf.RPICameraSaturation,
		Sharpness:         cnf.RPICameraSharpness,
		Exposure:          cnf.RPICameraExposure,
		AWB:               cnf.RPICameraAWB,
		Denoise:           cnf.RPICameraDenoise,
		Shutter:           cnf.RPICameraShutter,
		Metering:          cnf.RPICameraMetering,
		Gain:              cnf.RPICameraGain,
		EV:                cnf.RPICameraEV,
		ROI:               cnf.RPICameraROI,
		TuningFile:        cnf.RPICameraTuningFile,
		Mode:              cnf.RPICameraMode,
		FPS:               cnf.RPICameraFPS,
		IDRPeriod:         cnf.RPICameraIDRPeriod,
		Bitrate:           cnf.RPICameraBitrate,
		Profile:           cnf.RPICameraProfile,
		Level:             cnf.RPICameraLevel,
		AfMode:            cnf.RPICameraAfMode,
		AfRange:           cnf.RPICameraAfRange,
		AfSpeed:           cnf.RPICameraAfSpeed,
		LensPosition:      cnf.RPICameraLensPosition,
		AfWindow:          cnf.RPICameraAfWindow,
		TextOverlayEnable: cnf.RPICameraTextOverlayEnable,
		TextOverlay:       cnf.RPICameraTextOverlay,
	}
}

type rpiCameraSourceParent interface {
	logger.Writer
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rpiCameraSource struct {
	parent rpiCameraSourceParent
}

func newRPICameraSource(
	parent rpiCameraSourceParent,
) *rpiCameraSource {
	return &rpiCameraSource{
		parent: parent,
	}
}

func (s *rpiCameraSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[rpicamera source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rpiCameraSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	medi := &media.Media{
		Type: media.TypeVideo,
		Formats: []formats.Format{&formats.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}},
	}
	medias := media.Medias{medi}
	var stream *stream

	onData := func(dts time.Duration, au [][]byte) {
		if stream == nil {
			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				medias:             medias,
				generateRTPPackets: true,
			})
			if res.err != nil {
				return
			}

			s.Log(logger.Info, "ready: %s", sourceMediaInfo(medias))
			stream = res.stream
		}

		stream.writeUnit(medi, medi.Formats[0], &formatprocessor.UnitH264{
			PTS: dts,
			AU:  au,
			NTP: time.Now(),
		})
	}

	cam, err := rpicamera.New(paramsFromConf(cnf), onData)
	if err != nil {
		return err
	}
	defer cam.Close()

	defer func() {
		if stream != nil {
			s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	for {
		select {
		case cnf := <-reloadConf:
			cam.ReloadParams(paramsFromConf(cnf))

		case <-ctx.Done():
			return nil
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*rpiCameraSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rpiCameraSource"}
}
