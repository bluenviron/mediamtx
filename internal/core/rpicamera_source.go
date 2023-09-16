package core

import (
	"context"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/rpicamera"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
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
		HDR:               cnf.RPICameraHDR,
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
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
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
	s.parent.Log(level, "[RPI Camera source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rpiCameraSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	medi := &description.Media{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}},
	}
	medias := []*description.Media{medi}
	var stream *stream.Stream

	onData := func(dts time.Duration, au [][]byte) {
		if stream == nil {
			res := s.parent.setReady(pathSourceStaticSetReadyReq{
				desc:               &description.Session{Medias: medias},
				generateRTPPackets: true,
			})
			if res.err != nil {
				return
			}

			stream = res.stream
		}

		stream.WriteUnit(medi, medi.Formats[0], &unit.H264{
			Base: unit.Base{
				NTP: time.Now(),
				PTS: dts,
			},
			AU: au,
		})
	}

	cam, err := rpicamera.New(paramsFromConf(cnf), onData)
	if err != nil {
		return err
	}
	defer cam.Close()

	defer func() {
		if stream != nil {
			s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})
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
func (*rpiCameraSource) apiSourceDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "rpiCameraSource",
		ID:   "",
	}
}
