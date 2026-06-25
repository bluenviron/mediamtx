//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"encoding/base64"
	"reflect"
	"strconv"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type cameraParams struct {
	LogLevel          string
	CameraID          uint32
	Width             uint32
	Height            uint32
	HFlip             bool
	VFlip             bool
	Brightness        float32
	Contrast          float32
	Saturation        float32
	Sharpness         float32
	Exposure          string
	AWB               string
	AWBGainRed        float32
	AWBGainBlue       float32
	Denoise           string
	Shutter           uint32
	Metering          string
	Gain              float32
	EV                float32
	ROI               string
	HDR               bool
	TuningFile        string
	Mode              string
	FPS               float32
	AfMode            string
	AfRange           string
	AfSpeed           string
	LensPosition      float32
	AfWindow          string
	FlickerPeriod     uint32
	TextOverlayEnable bool
	TextOverlay       string
	Codec             string
	IDRPeriod         uint32
	Bitrate           uint32
	H264Profile       string
	H264Level         string

	SecondaryWidth        uint32
	SecondaryHeight       uint32
	SecondaryFPS          float32
	SecondaryMJPEGQuality uint32
}

func (p *cameraParams) fromConf(logLevel conf.LogLevel, cnf *conf.Path) {
	p.LogLevel = func() string {
		switch logLevel {
		case conf.LogLevel(logger.Debug):
			return "debug"
		case conf.LogLevel(logger.Info):
			return "info"
		case conf.LogLevel(logger.Warn):
			return "warn"
		}
		return "error"
	}()
	p.CameraID = uint32(cnf.RPICameraCamID)
	p.Width = uint32(cnf.RPICameraWidth)
	p.Height = uint32(cnf.RPICameraHeight)
	p.HFlip = cnf.RPICameraHFlip
	p.VFlip = cnf.RPICameraVFlip
	p.Brightness = float32(cnf.RPICameraBrightness)
	p.Contrast = float32(cnf.RPICameraContrast)
	p.Saturation = float32(cnf.RPICameraSaturation)
	p.Sharpness = float32(cnf.RPICameraSharpness)
	p.Exposure = cnf.RPICameraExposure
	p.AWB = cnf.RPICameraAWB
	p.AWBGainRed = float32(cnf.RPICameraAWBGains[0])
	p.AWBGainBlue = float32(cnf.RPICameraAWBGains[1])
	p.Denoise = cnf.RPICameraDenoise
	p.Shutter = uint32(cnf.RPICameraShutter)
	p.Metering = cnf.RPICameraMetering
	p.Gain = float32(cnf.RPICameraGain)
	p.EV = float32(cnf.RPICameraEV)
	p.ROI = cnf.RPICameraROI
	p.HDR = cnf.RPICameraHDR
	p.TuningFile = cnf.RPICameraTuningFile
	p.Mode = cnf.RPICameraMode
	p.FPS = float32(cnf.RPICameraFPS)
	p.AfMode = cnf.RPICameraAfMode
	p.AfRange = cnf.RPICameraAfRange
	p.AfSpeed = cnf.RPICameraAfSpeed
	p.LensPosition = float32(cnf.RPICameraLensPosition)
	p.AfWindow = cnf.RPICameraAfWindow
	p.FlickerPeriod = uint32(cnf.RPICameraFlickerPeriod)
	p.TextOverlayEnable = cnf.RPICameraTextOverlayEnable
	p.TextOverlay = cnf.RPICameraTextOverlay

	p.Codec = func() string {
		if cnf.RPICameraCodec == "auto" {
			if !cnf.RPICameraSecondary {
				if supportsHardwareH264() {
					return "hardwareH264"
				}
				return "softwareH264"
			}
			return "mjpeg"
		}
		return cnf.RPICameraCodec
	}()

	p.IDRPeriod = uint32(cnf.RPICameraIDRPeriod)
	p.Bitrate = uint32(cnf.RPICameraBitrate)

	p.H264Profile = func() string {
		if p.Codec == "hardwareH264" && cnf.RPICameraHardwareH264Profile != nil {
			return *cnf.RPICameraHardwareH264Profile
		}
		if p.Codec == "softwareH264" && cnf.RPICameraSoftwareH264Profile != nil {
			return *cnf.RPICameraSoftwareH264Profile
		}

		if cnf.RPICameraH264Profile == "auto" {
			if p.Codec == "hardwareH264" {
				return "main"
			}
			return "baseline"
		}

		return cnf.RPICameraH264Profile
	}()

	p.H264Level = func() string {
		if p.Codec == "hardwareH264" && cnf.RPICameraHardwareH264Level != nil {
			return *cnf.RPICameraHardwareH264Level
		}
		if p.Codec == "softwareH264" && cnf.RPICameraSoftwareH264Level != nil {
			return *cnf.RPICameraSoftwareH264Level
		}

		return cnf.RPICameraH264Level
	}()

	p.SecondaryWidth = uint32(cnf.RPICameraSecondaryWidth)
	p.SecondaryHeight = uint32(cnf.RPICameraSecondaryHeight)
	p.SecondaryFPS = float32(cnf.RPICameraSecondaryFPS)
	p.SecondaryMJPEGQuality = uint32(cnf.RPICameraSecondaryMJPEGQuality)
}

func (p cameraParams) serialize() []byte {
	rv := reflect.ValueOf(p)
	rt := rv.Type()
	nf := rv.NumField()
	ret := make([]string, nf)

	for i := range nf {
		entry := rt.Field(i).Name + ":"
		f := rv.Field(i)
		v := f.Interface()

		switch v := v.(type) {
		case uint32:
			entry += strconv.FormatUint(uint64(v), 10)

		case float32:
			entry += strconv.FormatFloat(float64(v), 'f', -1, 64)

		case string:
			entry += base64.StdEncoding.EncodeToString([]byte(v))

		case bool:
			if f.Bool() {
				entry += "1"
			} else {
				entry += "0"
			}

		default:
			panic("unhandled type")
		}

		ret[i] = entry
	}

	return []byte(strings.Join(ret, " "))
}
