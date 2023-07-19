package rpicamera

import (
	"encoding/base64"
	"reflect"
	"strconv"
	"strings"
)

// Params is a set of camera parameters.
type Params struct {
	CameraID          int
	Width             int
	Height            int
	HFlip             bool
	VFlip             bool
	Brightness        float64
	Contrast          float64
	Saturation        float64
	Sharpness         float64
	Exposure          string
	AWB               string
	Denoise           string
	Shutter           int
	Metering          string
	Gain              float64
	EV                float64
	ROI               string
	HDR               bool
	TuningFile        string
	Mode              string
	FPS               float64
	IDRPeriod         int
	Bitrate           int
	Profile           string
	Level             string
	AfMode            string
	AfRange           string
	AfSpeed           string
	LensPosition      float64
	AfWindow          string
	TextOverlayEnable bool
	TextOverlay       string
}

func (p Params) serialize() []byte { //nolint:unused
	rv := reflect.ValueOf(p)
	rt := rv.Type()
	nf := rv.NumField()
	ret := make([]string, nf)

	for i := 0; i < nf; i++ {
		entry := rt.Field(i).Name + ":"
		f := rv.Field(i)

		switch f.Kind() {
		case reflect.Int:
			entry += strconv.FormatInt(f.Int(), 10)

		case reflect.Float64:
			entry += strconv.FormatFloat(f.Float(), 'f', -1, 64)

		case reflect.String:
			entry += base64.StdEncoding.EncodeToString([]byte(f.String()))

		case reflect.Bool:
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
