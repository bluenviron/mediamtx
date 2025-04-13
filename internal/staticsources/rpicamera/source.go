// Package rpicamera contains the Raspberry Pi Camera static source.
package rpicamera

import (
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
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
	}
}

// Source is a Raspberry Pi Camera static source.
type Source struct {
	LogLevel conf.LogLevel
	Parent   defs.StaticSourceParent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[RPI Camera source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	medi := &description.Media{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}},
	}
	medias := []*description.Media{medi}
	var stream *stream.Stream

	onData := func(pts int64, ntp time.Time, au [][]byte) {
		if stream == nil {
			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:               &description.Session{Medias: medias},
				GenerateRTPPackets: true,
			})
			if res.Err != nil {
				return
			}

			stream = res.Stream
		}

		stream.WriteUnit(medi, medi.Formats[0], &unit.H264{
			Base: unit.Base{
				PTS: pts,
				NTP: ntp,
			},
			AU: au,
		})
	}

	defer func() {
		if stream != nil {
			s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})
		}
	}()

	cam := &camera{
		params: paramsFromConf(s.LogLevel, params.Conf),
		onData: onData,
	}
	err := cam.initialize()
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

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "rpiCameraSource",
		ID:   "",
	}
}
