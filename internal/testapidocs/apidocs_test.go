package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type openAPIProperty struct {
	Ref      string           `yaml:"$ref"`
	Type     string           `yaml:"type"`
	Format   string           `yaml:"format"`
	Nullable bool             `yaml:"nullable"`
	Items    *openAPIProperty `yaml:"items"`
}

type openAPISchema struct {
	Type       string                     `yaml:"type"`
	Properties map[string]openAPIProperty `yaml:"properties"`
}

type openAPI struct {
	Components struct {
		Schemas map[string]openAPISchema `yaml:"schemas"`
	} `yaml:"components"`
}

func fillProperty(t *testing.T, rt reflect.Type, existing openAPIProperty) openAPIProperty {
	switch {
	case rt == reflect.TypeOf(""):
		return openAPIProperty{Type: "string"}

	case rt == reflect.PointerTo(reflect.TypeOf("")):
		return openAPIProperty{
			Type:     "string",
			Nullable: true,
		}

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

	case rt == reflect.TypeOf(&time.Time{}):
		return openAPIProperty{Type: "string", Nullable: true}

	case rt == reflect.TypeOf(uuid.UUID{}):
		return openAPIProperty{Type: "string", Format: "uuid"}

	case rt == reflect.PointerTo(reflect.TypeOf(uuid.UUID{})):
		return openAPIProperty{Type: "string", Format: "uuid", Nullable: true}

	case rt == reflect.TypeOf(time.Time{}) ||
		rt == reflect.TypeOf(conf.Duration(0)) ||
		rt == reflect.TypeOf(conf.IPNetwork{}) ||
		rt == reflect.TypeOf(conf.Credential("")) ||
		rt == reflect.TypeOf(conf.RecordFormat(0)) ||
		rt == reflect.TypeOf(conf.AuthAction("")) ||
		rt == reflect.TypeOf(conf.Encryption(0)) ||
		rt == reflect.TypeOf(conf.RTSPTransport{}) ||
		rt == reflect.TypeOf(conf.StringSize(0)) ||
		rt == reflect.TypeOf(conf.RTSPRangeType(0)) ||
		rt == reflect.TypeOf(conf.LogLevel(0)) ||
		rt == reflect.TypeOf(conf.AuthMethod(0)) ||
		rt == reflect.TypeOf(conf.LogDestination(0)) ||
		rt == reflect.TypeOf(conf.RTSPAuthMethod(0)) ||
		rt == reflect.TypeOf(conf.HLSVariant(0)) ||
		rt == reflect.TypeOf(defs.APIRTMPConnState("")) ||
		rt == reflect.TypeOf(defs.APIWebRTCSessionState("")) ||
		rt == reflect.TypeOf(defs.APISRTConnState("")) ||
		rt == reflect.TypeOf(defs.APIRTSPSessionState("")):
		return openAPIProperty{Type: "string"}

	case rt == reflect.TypeOf(conf.RTSPTransports{}):
		return openAPIProperty{
			Type: "array",
			Items: &openAPIProperty{
				Type: "string",
			},
		}

	case rt.Kind() == reflect.Struct:
		schemaName := strings.TrimPrefix(rt.Name(), "API")
		if rt.PkgPath() == "github.com/bluenviron/mediamtx/internal/conf" && schemaName == "Path" {
			schemaName = "PathConf"
		}

		return openAPIProperty{
			Ref: "#/components/schemas/" + schemaName,
		}

	case rt.Kind() == reflect.Pointer && rt.Elem().Kind() == reflect.Struct:
		prop := fillProperty(t, rt.Elem(), existing)
		prop.Nullable = true
		return prop

	case rt.Kind() == reflect.Slice:
		items := fillProperty(t, rt.Elem(), *existing.Items)
		return openAPIProperty{
			Type:  "array",
			Items: &items,
		}

	default:
		t.Errorf("unhandled type: %v", rt)
		return openAPIProperty{}
	}
}

func TestAPIDocs(t *testing.T) {
	byts, err := os.ReadFile("../../api/openapi.yaml")
	require.NoError(t, err)

	var doc openAPI
	err = yaml.Unmarshal(byts, &doc)
	require.NoError(t, err)

	for _, ca := range []struct {
		openAPIKey string
		goStruct   any
	}{
		{
			"Info",
			defs.APIInfo{},
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
			"PathConf",
			conf.Path{},
		},
		{
			"PathConfList",
			defs.APIPathConfList{},
		},
		{
			"Path",
			defs.APIPath{},
		},
		{
			"PathList",
			defs.APIPathList{},
		},
		{
			"PathSource",
			defs.APIPathSource{},
		},
		{
			"PathReader",
			defs.APIPathReader{},
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

				if js != "-" && js != "paths" && js != "pathDefaults" && !strings.Contains(js, ",omitempty") {
					content2.Properties[js] = fillProperty(t, sf.Type, content1.Properties[js])
				}
			}

			require.Equal(t, content2, content1)
		})
	}
}
