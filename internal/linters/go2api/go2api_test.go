//go:build enable_linters

package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/goccy/go-yaml"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type openAPIProperty struct {
	Ref        string            `yaml:"$ref"`
	Type       string            `yaml:"type"`
	Format     string            `yaml:"format"`
	AllOf      []openAPIProperty `yaml:"allOf"`
	Nullable   bool              `yaml:"nullable"`
	Deprecated bool              `yaml:"deprecated"`
	Enum       []string          `yaml:"enum"`
	Items      *openAPIProperty  `yaml:"items"`
}

func wrapRef(rt reflect.Type, p openAPIProperty) openAPIProperty {
	if p.Ref == "" {
		return p
	}

	if _, ok := goEnumToApi(rt); ok {
		p.Type = "string"
	} else if rt.Kind() == reflect.Struct {
		p.Type = "object"
	}

	p.AllOf = []openAPIProperty{{Ref: p.Ref}}
	p.Ref = ""
	return p
}

type openAPISchema struct {
	Type       string                     `yaml:"type"`
	Enum       []string                   `yaml:"enum"`
	Properties map[string]openAPIProperty `yaml:"properties"`
}

type openAPI struct {
	Components struct {
		Schemas map[string]openAPISchema `yaml:"schemas"`
	} `yaml:"components"`
}

func schemaName(rt reflect.Type) string {
	name := strings.TrimPrefix(rt.Name(), "API")

	if rt.PkgPath() == "github.com/bluenviron/mediamtx/internal/conf" && name == "Path" {
		return "PathConf"
	}

	return name
}

func goStructToApi(t *testing.T, rt reflect.Type) openAPIProperty {
	if rt.Kind() == reflect.Pointer {
		prop := goStructToApi(t, rt.Elem())
		prop = wrapRef(rt.Elem(), prop)
		prop.Nullable = true
		return prop
	}

	if _, ok := goEnumToApi(rt); ok {
		return openAPIProperty{Ref: "#/components/schemas/" + schemaName(rt)}
	}

	switch {
	case rt == reflect.TypeOf(""):
		return openAPIProperty{Type: "string"}

	case rt == reflect.TypeOf(int(0)):
		return openAPIProperty{Type: "integer", Format: "int64"}

	case rt == reflect.TypeOf(uint(0)):
		return openAPIProperty{Type: "integer", Format: "uint64"}

	case rt == reflect.TypeOf(uint64(0)):
		return openAPIProperty{Type: "integer", Format: "uint64"}

	case rt == reflect.TypeOf(float64(0)):
		return openAPIProperty{Type: "number", Format: "double"}

	case rt == reflect.TypeOf(false):
		return openAPIProperty{Type: "boolean"}

	case rt == reflect.TypeOf(uuid.UUID{}):
		return openAPIProperty{Type: "string", Format: "uuid"}

	case rt == reflect.TypeOf(time.Time{}) ||
		rt == reflect.TypeOf(conf.Duration(0)) ||
		rt == reflect.TypeOf(conf.IPNetwork{}) ||
		rt == reflect.TypeOf(conf.Credential("")) ||
		rt == reflect.TypeOf(conf.StringSize(0)):
		return openAPIProperty{Type: "string"}

	case rt == reflect.TypeOf(conf.RTSPTransports{}):
		return openAPIProperty{
			Type: "array",
			Items: &openAPIProperty{
				Type: "string",
				Enum: []string{"udp", "multicast", "tcp"},
			},
		}

	case rt.Kind() == reflect.Struct:
		return openAPIProperty{
			Ref: "#/components/schemas/" + schemaName(rt),
		}

	case rt.Kind() == reflect.Slice:
		items := goStructToApi(t, rt.Elem())
		return openAPIProperty{
			Type:  "array",
			Items: &items,
		}

	default:
		t.Errorf("unhandled type: %v", rt)
		return openAPIProperty{}
	}
}

func goEnumToApi(rt reflect.Type) (openAPISchema, bool) {
	switch rt {
	case reflect.TypeOf(defs.APIOKStatus("")):
		return openAPISchema{Type: "string", Enum: []string{"ok"}}, true

	case reflect.TypeOf(defs.APIErrorStatus("")):
		return openAPISchema{Type: "string", Enum: []string{"error"}}, true

	case reflect.TypeOf(defs.APIPathSourceType("")):
		return openAPISchema{Type: "string", Enum: []string{
			"hlsSource",
			"redirect",
			"rpiCameraSource",
			"rtmpConn",
			"rtmpsConn",
			"rtmpSource",
			"rtspSession",
			"rtspSource",
			"rtspsSession",
			"srtConn",
			"srtSource",
			"mpegtsSource",
			"rtpSource",
			"webRTCSession",
			"webRTCSource",
		}}, true

	case reflect.TypeOf(defs.APIPathReaderType("")):
		return openAPISchema{Type: "string", Enum: []string{
			"hlsMuxer",
			"rpiCameraSecondary",
			"rtmpConn",
			"rtmpsConn",
			"rtspConn",
			"rtspSession",
			"rtspsConn",
			"rtspsSession",
			"srtConn",
			"webRTCSession",
		}}, true

	case reflect.TypeOf(defs.APIPathTrackCodec("")):
		return openAPISchema{Type: "string", Enum: []string{
			"AV1",
			"VP9",
			"VP8",
			"H265",
			"H264",
			"MPEG-4 Video",
			"MPEG-1 Video",
			"MJPEG",
			"Opus",
			"Vorbis",
			"MPEG-4 Audio",
			"MPEG-4 Audio LATM",
			"MPEG-1 Audio",
			"AC3",
			"Speex",
			"G726",
			"G722",
			"G711",
			"LPCM",
			"MPEG-TS",
			"KLV",
			"Generic",
		}}, true

	case reflect.TypeOf(conf.AlwaysAvailableTrackCodec("")):
		return openAPISchema{Type: "string", Enum: []string{
			"AV1",
			"VP9",
			"H265",
			"H264",
			"MPEG4Audio",
			"Opus",
			"G711",
			"LPCM",
		}}, true

	case reflect.TypeOf(conf.AuthAction("")):
		return openAPISchema{Type: "string", Enum: []string{
			"publish",
			"read",
			"playback",
			"api",
			"metrics",
			"pprof",
		}}, true

	case reflect.TypeOf(conf.AuthMethod("")):
		return openAPISchema{Type: "string", Enum: []string{
			"internal",
			"http",
			"jwt",
		}}, true

	case reflect.TypeOf(conf.Encryption("")):
		return openAPISchema{Type: "string", Enum: []string{
			"no",
			"optional",
			"strict",
		}}, true

	case reflect.TypeOf(conf.HLSVariant(0)):
		return openAPISchema{Type: "string", Enum: []string{
			"mpegts",
			"fmp4",
			"lowLatency",
		}}, true

	case reflect.TypeOf(conf.LogDestination(0)):
		return openAPISchema{Type: "string", Enum: []string{
			"stdout",
			"file",
			"syslog",
		}}, true

	case reflect.TypeOf(conf.LogLevel(0)):
		return openAPISchema{Type: "string", Enum: []string{
			"error",
			"warn",
			"info",
			"debug",
		}}, true

	case reflect.TypeOf(conf.RecordFormat("")):
		return openAPISchema{Type: "string", Enum: []string{
			"fmp4",
			"mpegts",
		}}, true

	case reflect.TypeOf(conf.RTSPAuthMethod(0)):
		return openAPISchema{Type: "string", Enum: []string{
			"basic",
			"digest",
		}}, true

	case reflect.TypeOf(conf.RTSPRangeType("")):
		return openAPISchema{Type: "string", Enum: []string{
			"",
			"clock",
			"npt",
			"smpte",
		}}, true

	case reflect.TypeOf(conf.RTSPTransport{}):
		return openAPISchema{Type: "string", Enum: []string{
			"udp",
			"multicast",
			"tcp",
			"automatic",
		}}, true

	case reflect.TypeOf(defs.APIRTMPConnState("")):
		return openAPISchema{Type: "string", Enum: []string{"idle", "read", "publish"}}, true

	case reflect.TypeOf(defs.APIWebRTCSessionState("")):
		return openAPISchema{Type: "string", Enum: []string{"read", "publish"}}, true

	case reflect.TypeOf(defs.APISRTConnState("")):
		return openAPISchema{Type: "string", Enum: []string{"idle", "read", "publish"}}, true

	case reflect.TypeOf(defs.APIRTSPSessionState("")):
		return openAPISchema{Type: "string", Enum: []string{"idle", "read", "publish"}}, true
	}

	return openAPISchema{}, false
}

func TestGo2API(t *testing.T) {
	byts, err := os.ReadFile("../../../api/openapi.yaml")
	require.NoError(t, err)

	var doc openAPI
	err = yaml.Unmarshal(byts, &doc)
	require.NoError(t, err)

	t.Run("structs", func(t *testing.T) {
		for _, ca := range []struct {
			openAPIKey string
			goStruct   any
		}{
			{
				"AlwaysAvailableTrack",
				conf.AlwaysAvailableTrack{},
			},
			{
				"AuthInternalUser",
				conf.AuthInternalUser{},
			},
			{
				"AuthInternalUserPermission",
				conf.AuthInternalUserPermission{},
			},
			{
				"GlobalConf",
				conf.Conf{},
			},
			{
				"HLSMuxer",
				defs.APIHLSMuxer{},
			},
			{
				"HLSMuxerList",
				defs.APIHLSMuxerList{},
			},
			{
				"Info",
				defs.APIInfo{},
			},
			{
				"Path",
				defs.APIPath{},
			},
			{
				"PathConf",
				conf.Path{},
			},
			{
				"PathConfList",
				defs.APIPathConfList{},
			},
			{
				"PathList",
				defs.APIPathList{},
			},
			{
				"PathReader",
				defs.APIPathReader{},
			},
			{
				"PathSource",
				defs.APIPathSource{},
			},
			{
				"Recording",
				defs.APIRecording{},
			},
			{
				"RecordingList",
				defs.APIRecordingList{},
			},
			{
				"RecordingSegment",
				defs.APIRecordingSegment{},
			},
			{
				"RTMPConn",
				defs.APIRTMPConn{},
			},
			{
				"RTMPConnList",
				defs.APIRTMPConnList{},
			},
			{
				"RTSPConn",
				defs.APIRTSPConn{},
			},
			{
				"RTSPConnList",
				defs.APIRTSPConnsList{},
			},
			{
				"RTSPSession",
				defs.APIRTSPSession{},
			},
			{
				"RTSPSessionList",
				defs.APIRTSPSessionList{},
			},
			{
				"SRTConn",
				defs.APISRTConn{},
			},
			{
				"SRTConnList",
				defs.APISRTConnList{},
			},
			{
				"WebRTCSession",
				defs.APIWebRTCSession{},
			},
			{
				"WebRTCSessionList",
				defs.APIWebRTCSessionList{},
			},
		} {
			t.Run(ca.openAPIKey, func(t *testing.T) {
				content1 := doc.Components.Schemas[ca.openAPIKey]

				content2 := openAPISchema{
					Type:       "object",
					Properties: make(map[string]openAPIProperty),
				}

				ty := reflect.TypeOf(ca.goStruct)

				for i := range ty.NumField() {
					sf := ty.Field(i)
					js := sf.Tag.Get("json")
					name, _, _ := strings.Cut(js, ",")
					deprecated := sf.Tag.Get("deprecated") == "true"

					if name != "" && name != "-" && name != "paths" && name != "pathDefaults" &&
						(!strings.Contains(js, ",omitempty") || deprecated) {
						prop := goStructToApi(t, sf.Type)
						prop.Deprecated = deprecated
						if deprecated {
							prop = wrapRef(sf.Type, prop)
						}
						content2.Properties[name] = prop
					}
				}

				require.Equal(t, content2, content1)
			})
		}
	})

	t.Run("enums", func(t *testing.T) {
		for _, rt := range []reflect.Type{
			reflect.TypeOf(defs.APIOKStatus("")),
			reflect.TypeOf(defs.APIErrorStatus("")),
			reflect.TypeOf(defs.APIPathSourceType("")),
			reflect.TypeOf(defs.APIPathReaderType("")),
			reflect.TypeOf(defs.APIPathTrackCodec("")),
			reflect.TypeOf(conf.AlwaysAvailableTrackCodec("")),
			reflect.TypeOf(conf.AuthAction("")),
			reflect.TypeOf(conf.AuthMethod("")),
			reflect.TypeOf(conf.Encryption("")),
			reflect.TypeOf(conf.HLSVariant(0)),
			reflect.TypeOf(conf.LogDestination(0)),
			reflect.TypeOf(conf.LogLevel(0)),
			reflect.TypeOf(conf.RecordFormat("")),
			reflect.TypeOf(conf.RTSPAuthMethod(0)),
			reflect.TypeOf(conf.RTSPRangeType("")),
			reflect.TypeOf(conf.RTSPTransport{}),
			reflect.TypeOf(defs.APIRTMPConnState("")),
			reflect.TypeOf(defs.APIRTSPSessionState("")),
			reflect.TypeOf(defs.APISRTConnState("")),
			reflect.TypeOf(defs.APIWebRTCSessionState("")),
		} {
			t.Run(rt.Name(), func(t *testing.T) {
				content1 := doc.Components.Schemas[schemaName(rt)]
				content2, ok := goEnumToApi(rt)
				require.True(t, ok)
				require.Equal(t, content2, content1)
			})
		}
	})
}
