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

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

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
		AWBGainRed:        cnf.RPICameraAWBGains[0],
		AWBGainBlue:       cnf.RPICameraAWBGains[1],
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
		AfMode:            cnf.RPICameraAfMode,
		AfRange:           cnf.RPICameraAfRange,
		AfSpeed:           cnf.RPICameraAfSpeed,
		LensPosition:      cnf.RPICameraLensPosition,
		AfWindow:          cnf.RPICameraAfWindow,
		FlickerPeriod:     cnf.RPICameraFlickerPeriod,
		TextOverlayEnable: cnf.RPICameraTextOverlayEnable,
		TextOverlay:       cnf.RPICameraTextOverlay,
		Codec:             cnf.RPICameraCodec,
		IDRPeriod:         cnf.RPICameraIDRPeriod,
		Bitrate:           cnf.RPICameraBitrate,
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

	onData := func(dts time.Duration, au [][]byte) {
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
				NTP: time.Now(),
				PTS: multiplyAndDivide(int64(dts), 90000, int64(time.Second)),
			},
			AU: au,
		})
	}

	cam := &camera{
		Params: paramsFromConf(s.LogLevel, params.Conf),
		OnData: onData,
	}
	err := cam.initialize()
	if err != nil {
		return err
	}
	defer cam.close()

	defer func() {
		if stream != nil {
			s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})
		}
	}()

	for {
		select {
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
