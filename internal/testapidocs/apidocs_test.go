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

type openAPIProperty struct {
	Ref      string           `json:"$ref"`
	Type     string           `json:"type"`
	Nullable bool             `json:"nullable"`
	Items    *openAPIProperty `json:"items"`
}

type openAPISchema struct {
	Type       string                     `json:"type"`
	Properties map[string]openAPIProperty `json:"properties"`
}

type openAPI struct {
	Components struct {
		Schemas map[string]openAPISchema `json:"schemas"`
	} `json:"components"`
}

func TestAPIDocs(t *testing.T) {
	byts, err := os.ReadFile("../../apidocs/openapi.yaml")
	require.NoError(t, err)

	var doc openAPI
	err = yamlwrapper.Unmarshal(byts, &doc)
	require.NoError(t, err)

	for _, ca := range []struct {
		openAPIKey string
		goStruct   any
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
					switch {
					case sf.Type == reflect.TypeOf(""):
						content2.Properties[js] = openAPIProperty{Type: "string"}

					case sf.Type == reflect.TypeOf(int(0)):
						content2.Properties[js] = openAPIProperty{Type: "integer"}

					case sf.Type == reflect.TypeOf(false):
						content2.Properties[js] = openAPIProperty{Type: "boolean"}

					case sf.Type == reflect.TypeOf(time.Time{}):
						content2.Properties[js] = openAPIProperty{Type: "string"}

					case sf.Type == reflect.TypeOf(&time.Time{}):
						content2.Properties[js] = openAPIProperty{
							Type:     "string",
							Nullable: true,
						}

					case sf.Type == reflect.TypeOf(conf.AuthInternalUserPermissions{}):
						content2.Properties[js] = openAPIProperty{
							Type: "array",
							Items: &openAPIProperty{
								Ref: "#/components/schemas/AuthInternalUserPermission",
							},
						}

					default:
						if existing, ok := content1.Properties[js]; ok {
							content2.Properties[js] = existing
						} else {
							t.Errorf("missing item: '%s'", js)
						}
					}
				}
			}

			require.Equal(t, content2, content1)
		})
	}
}
