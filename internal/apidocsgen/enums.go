// Package main contains a tool to generate openapi.yaml.
package main

import (
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

var enums = []struct {
	externalName string
	internalName string
	File         string
}{
	{
		externalName: "ErrorStatus",
		internalName: "APIErrorStatus",
		File:         filepath.Join("internal", "defs", "api.go"),
	},
	{
		externalName: "AlwaysAvailableTrackCodec",
		internalName: "AlwaysAvailableTrackCodec",
		File:         filepath.Join("internal", "conf", "always_available_track_codec.go"),
	},
	{
		externalName: "AuthAction",
		internalName: "AuthAction",
		File:         filepath.Join("internal", "conf", "auth_action.go"),
	},
	{
		externalName: "AuthMethod",
		internalName: "AuthMethod",
		File:         filepath.Join("internal", "conf", "auth_method.go"),
	},
	{
		externalName: "Encryption",
		internalName: "Encryption",
		File:         filepath.Join("internal", "conf", "encryption.go"),
	},
	{
		externalName: "MoQSessionState",
		internalName: "APIMoQSessionState",
		File:         filepath.Join("internal", "defs", "api_moq.go"),
	},
	{
		externalName: "OKStatus",
		internalName: "APIOKStatus",
		File:         filepath.Join("internal", "defs", "api.go"),
	},
	{
		externalName: "PathReaderType",
		internalName: "APIPathReaderType",
		File:         filepath.Join("internal", "defs", "api_path.go"),
	},
	{
		externalName: "PathSourceType",
		internalName: "APIPathSourceType",
		File:         filepath.Join("internal", "defs", "api_path.go"),
	},
	{
		externalName: "PushTargetProtocol",
		internalName: "APIPushTargetProtocol",
		File:         filepath.Join("internal", "defs", "api_push_target.go"),
	},
	{
		externalName: "PushTargetSource",
		internalName: "APIPushTargetSource",
		File:         filepath.Join("internal", "defs", "api_push_target.go"),
	},
	{
		externalName: "PushTargetState",
		internalName: "APIPushTargetState",
		File:         filepath.Join("internal", "defs", "api_push_target.go"),
	},
	{
		externalName: "PathTrackCodec",
		internalName: "Label",
		File:         filepath.Join("internal", "formatlabel", "label.go"),
	},
	{
		externalName: "RTMPConnState",
		internalName: "APIRTMPConnState",
		File:         filepath.Join("internal", "defs", "api_rtmp.go"),
	},
	{
		externalName: "RTSPRangeType",
		internalName: "RTSPRangeType",
		File:         filepath.Join("internal", "conf", "rtsp_range_type.go"),
	},
	{
		externalName: "RTSPSessionState",
		internalName: "APIRTSPSessionState",
		File:         filepath.Join("internal", "defs", "api_rtsp.go"),
	},
	{
		externalName: "RecordFormat",
		internalName: "RecordFormat",
		File:         filepath.Join("internal", "conf", "record_format.go"),
	},
	{
		externalName: "SRTConnState",
		internalName: "APISRTConnState",
		File:         filepath.Join("internal", "defs", "api_srt.go"),
	},
	{
		externalName: "WebRTCSessionState",
		internalName: "APIWebRTCSessionState",
		File:         filepath.Join("internal", "defs", "api_webrtc.go"),
	},
}

func extractEnumValues(name, file string) ([]string, error) {
	fset := gotoken.NewFileSet()

	f, err := goparser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return nil, err
	}

	var values []string

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*goast.GenDecl)
		if !ok || genDecl.Tok != gotoken.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			var valSpec *goast.ValueSpec
			valSpec, ok = spec.(*goast.ValueSpec)
			if !ok || valSpec.Type == nil {
				continue
			}

			var ident *goast.Ident
			ident, ok = valSpec.Type.(*goast.Ident)
			if !ok || ident.Name != name {
				continue
			}

			for _, val := range valSpec.Values {
				var lit *goast.BasicLit
				lit, ok = val.(*goast.BasicLit)
				if !ok {
					continue
				}
				values = append(values, strings.Trim(lit.Value, `"`))
			}
		}
	}

	return values, nil
}

func addEnums(astFile *ast.File) error {
	for _, e := range enums {
		values, err := extractEnumValues(e.internalName, e.File)
		if err != nil {
			return err
		}

		schema := &openAPISchema{
			Type: "string",
			Enum: values,
		}

		schemaNode, err := yaml.ValueToNode(map[string]*openAPISchema{e.externalName: schema})
		if err != nil {
			return err
		}

		indentBlockSequences(schemaNode)

		addBlankLineBeforeNewSchemaEntry(schemaNode)

		schemasPath, err := yaml.PathString("$.components.schemas")
		if err != nil {
			return err
		}

		err = schemasPath.MergeFromNode(astFile, schemaNode)
		if err != nil {
			return err
		}
	}

	return nil
}
