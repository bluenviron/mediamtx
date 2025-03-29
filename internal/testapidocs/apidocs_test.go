package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/yamlwrapper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/stretchr/testify/require"
)

type openApiProperty struct {
	Ref      string `json:"$ref"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

type openApiSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]openApiProperty `json:"properties"`
}

type openApi struct {
	Components struct {
		Schemas map[string]openApiSchema `json:"schemas"`
	} `json:"components"`
}

func TestAPIDocs(t *testing.T) {
	byts, err := os.ReadFile("../../apidocs/openapi.yaml")
	require.NoError(t, err)

	var openApi openApi
	err = yamlwrapper.Unmarshal(byts, &openApi)
	require.NoError(t, err)

	for _, ca := range []struct {
		openApiKey string
		goStruct   interface{}
	}{
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
			defs.APIPathSourceOrReader{},
		},
		{
			"PathReader",
			defs.APIPathSourceOrReader{},
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
		t.Run(ca.openApiKey, func(t *testing.T) {
			content1 := openApi.Components.Schemas[ca.openApiKey]

			content2 := openApiSchema{
				Type:       "object",
				Properties: make(map[string]openApiProperty),
			}
			ty := reflect.TypeOf(ca.goStruct)
			for i := range ty.NumField() {
				sf := ty.Field(i)
				js := sf.Tag.Get("json")
				if js != "-" && js != "paths" && js != "pathDefaults" && !strings.Contains(js, ",omitempty") {
					switch {
					case sf.Type == reflect.TypeOf(""):
						content2.Properties[js] = openApiProperty{Type: "string"}

					case sf.Type == reflect.TypeOf(int(0)):
						content2.Properties[js] = openApiProperty{Type: "integer"}

					case sf.Type == reflect.TypeOf(false):
						content2.Properties[js] = openApiProperty{Type: "boolean"}

					case sf.Type == reflect.TypeOf(time.Time{}):
						content2.Properties[js] = openApiProperty{Type: "string"}

					case sf.Type == reflect.TypeOf(&time.Time{}):
						content2.Properties[js] = openApiProperty{
							Type:     "string",
							Nullable: true,
						}

					default:
						if existing, ok := content1.Properties[js]; ok {
							content2.Properties[js] = existing
						}
					}
				}
			}

			require.Equal(t, content2, content1)
		})
	}
}
