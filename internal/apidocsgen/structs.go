package main

import (
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/google/uuid"
)

var structs = []struct {
	externalName string
	typ          reflect.Type
}{
	{
		externalName: "AlwaysAvailableTrack",
		typ:          reflect.TypeOf(conf.AlwaysAvailableTrack{}),
	},
	{
		externalName: "AuthInternalUser",
		typ:          reflect.TypeOf(conf.AuthInternalUser{}),
	},
	{
		externalName: "AuthInternalUserPermission",
		typ:          reflect.TypeOf(conf.AuthInternalUserPermission{}),
	},
	{
		externalName: "Error",
		typ:          reflect.TypeOf(defs.APIError{}),
	},
	{
		externalName: "GlobalConf",
		typ:          reflect.TypeOf(conf.Conf{}),
	},
	{
		externalName: "HLSMuxer",
		typ:          reflect.TypeOf(defs.APIHLSMuxer{}),
	},
	{
		externalName: "HLSMuxerList",
		typ:          reflect.TypeOf(defs.APIHLSMuxerList{}),
	},
	{
		externalName: "HLSSession",
		typ:          reflect.TypeOf(defs.APIHLSSession{}),
	},
	{
		externalName: "HLSSessionList",
		typ:          reflect.TypeOf(defs.APIHLSSessionList{}),
	},
	{
		externalName: "Info",
		typ:          reflect.TypeOf(defs.APIInfo{}),
	},
	{
		externalName: "MoQSession",
		typ:          reflect.TypeOf(defs.APIMoQSession{}),
	},
	{
		externalName: "MoQSessionList",
		typ:          reflect.TypeOf(defs.APIMoQSessionList{}),
	},
	{
		externalName: "OK",
		typ:          reflect.TypeOf(defs.APIOK{}),
	},
	{
		externalName: "Path",
		typ:          reflect.TypeOf(defs.APIPath{}),
	},
	{
		externalName: "PathConf",
		typ:          reflect.TypeOf(conf.Path{}),
	},
	{
		externalName: "PathConfList",
		typ:          reflect.TypeOf(defs.APIPathConfList{}),
	},
	{
		externalName: "PathConfPushTarget",
		typ:          reflect.TypeOf(conf.PushTarget{}),
	},
	{
		externalName: "PathList",
		typ:          reflect.TypeOf(defs.APIPathList{}),
	},
	{
		externalName: "PathReader",
		typ:          reflect.TypeOf(defs.APIPathReader{}),
	},
	{
		externalName: "PathSource",
		typ:          reflect.TypeOf(defs.APIPathSource{}),
	},
	{
		externalName: "PathTrack",
		typ:          reflect.TypeOf(defs.APIPathTrack{}),
	},
	{
		externalName: "PathTrackCodecPropsAC3",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsAC3{}),
	},
	{
		externalName: "PathTrackCodecPropsAV1",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsAV1{}),
	},
	{
		externalName: "PathTrackCodecPropsG711",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsG711{}),
	},
	{
		externalName: "PathTrackCodecPropsH264",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsH264{}),
	},
	{
		externalName: "PathTrackCodecPropsH265",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsH265{}),
	},
	{
		externalName: "PathTrackCodecPropsLPCM",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsLPCM{}),
	},
	{
		externalName: "PathTrackCodecPropsMPEG4Audio",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsMPEG4Audio{}),
	},
	{
		externalName: "PathTrackCodecPropsOpus",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsOpus{}),
	},
	{
		externalName: "PathTrackCodecPropsVP9",
		typ:          reflect.TypeOf(defs.APIPathTrackCodecPropsVP9{}),
	},
	{
		externalName: "PushTarget",
		typ:          reflect.TypeOf(defs.APIPushTarget{}),
	},
	{
		externalName: "PushTargetAdd",
		typ:          reflect.TypeOf(defs.APIPushTargetAdd{}),
	},
	{
		externalName: "PushTargetList",
		typ:          reflect.TypeOf(defs.APIPushTargetList{}),
	},
	{
		externalName: "Recording",
		typ:          reflect.TypeOf(defs.APIRecording{}),
	},
	{
		externalName: "RecordingList",
		typ:          reflect.TypeOf(defs.APIRecordingList{}),
	},
	{
		externalName: "RecordingSegment",
		typ:          reflect.TypeOf(defs.APIRecordingSegment{}),
	},
	{
		externalName: "RTMPConn",
		typ:          reflect.TypeOf(defs.APIRTMPConn{}),
	},
	{
		externalName: "RTMPConnList",
		typ:          reflect.TypeOf(defs.APIRTMPConnList{}),
	},
	{
		externalName: "RTSPConn",
		typ:          reflect.TypeOf(defs.APIRTSPConn{}),
	},
	{
		externalName: "RTSPConnList",
		typ:          reflect.TypeOf(defs.APIRTSPConnsList{}),
	},
	{
		externalName: "RTSPSession",
		typ:          reflect.TypeOf(defs.APIRTSPSession{}),
	},
	{
		externalName: "RTSPSessionList",
		typ:          reflect.TypeOf(defs.APIRTSPSessionList{}),
	},
	{
		externalName: "SRTConn",
		typ:          reflect.TypeOf(defs.APISRTConn{}),
	},
	{
		externalName: "SRTConnList",
		typ:          reflect.TypeOf(defs.APISRTConnList{}),
	},
	{
		externalName: "WebRTCICEServer",
		typ:          reflect.TypeOf(conf.WebRTCICEServer{}),
	},
	{
		externalName: "WebRTCSession",
		typ:          reflect.TypeOf(defs.APIWebRTCSession{}),
	},
	{
		externalName: "WebRTCSessionList",
		typ:          reflect.TypeOf(defs.APIWebRTCSessionList{}),
	},
}

const modulePathPrefix = "github.com/bluenviron/mediamtx/"

func wrapRef(rt reflect.Type, p openAPIProperty) openAPIProperty {
	if p.Ref == "" {
		return p
	}

	if isStructEnum(rt) {
		p.Type = "string"
	} else if rt.Kind() == reflect.Struct {
		p.Type = "object"
	}

	p.AllOf = []openAPIProperty{{Ref: p.Ref}}
	p.Ref = ""
	return p
}

func goTypeToOpenAPI(rt reflect.Type) (openAPIProperty, error) {
	if rt.Kind() == reflect.Pointer {
		prop, err := goTypeToOpenAPI(rt.Elem())
		if err != nil {
			return openAPIProperty{}, err
		}

		prop = wrapRef(rt.Elem(), prop)
		prop.Nullable = true
		return prop, nil
	}

	if isStructEnum(rt) {
		return openAPIProperty{Ref: "#/components/schemas/" + schemaName(rt)}, nil
	}

	if rt == reflect.TypeOf((*defs.APIPathTrackCodecProps)(nil)).Elem() {
		return openAPIProperty{
			Type:     "object",
			AllOf:    []openAPIProperty{{Ref: "#/components/schemas/" + schemaName(rt)}},
			Nullable: true,
		}, nil
	}

	switch {
	case rt == reflect.TypeOf(uuid.UUID{}):
		return openAPIProperty{Type: "string", Format: "uuid"}, nil

	case rt == reflect.TypeOf(time.Time{}):
		return openAPIProperty{Type: "string"}, nil

	case rt == reflect.TypeOf(conf.Duration(0)):
		return openAPIProperty{Type: "string"}, nil

	case rt == reflect.TypeOf(conf.IPNetwork{}):
		return openAPIProperty{Type: "string"}, nil

	case rt == reflect.TypeOf(conf.StringSize(0)):
		return openAPIProperty{Type: "string"}, nil

	case rt == reflect.TypeOf(conf.RTSPTransports{}):
		items := openAPIProperty{Type: "string", Enum: []string{"udp", "multicast", "tcp"}}
		return openAPIProperty{Type: "array", Items: &items}, nil

	case rt.Kind() == reflect.String:
		return openAPIProperty{Type: "string"}, nil

	case rt.Kind() >= reflect.Int && rt.Kind() <= reflect.Int64:
		return openAPIProperty{Type: "integer", Format: "int64"}, nil

	case rt.Kind() >= reflect.Uint && rt.Kind() <= reflect.Uint64:
		return openAPIProperty{Type: "integer", Format: "uint64"}, nil

	case rt.Kind() == reflect.Float32 || rt.Kind() == reflect.Float64:
		return openAPIProperty{Type: "number", Format: "double"}, nil

	case rt.Kind() == reflect.Bool:
		return openAPIProperty{Type: "boolean"}, nil

	case rt.Kind() == reflect.Struct:
		return openAPIProperty{Ref: "#/components/schemas/" + schemaName(rt)}, nil

	case rt.Kind() == reflect.Slice:
		items, err := goTypeToOpenAPI(rt.Elem())
		if err != nil {
			return openAPIProperty{}, err
		}

		return openAPIProperty{Type: "array", Items: &items}, nil

	default:
		return openAPIProperty{}, fmt.Errorf("unhandled type: %s", rt.String())
	}
}

func schemaName(rt reflect.Type) string {
	if rt == reflect.TypeOf(conf.Path{}) {
		return "PathConf"
	}
	if rt == reflect.TypeOf(conf.PushTarget{}) {
		return "PathConfPushTarget"
	}

	if rt == reflect.TypeOf(defs.APIPathTrackCodec("")) {
		return "PathTrackCodec"
	}

	if rt == reflect.TypeOf((*defs.APIPathTrackCodecProps)(nil)).Elem() {
		return "PathTrackCodecProps"
	}

	return strings.TrimPrefix(rt.Name(), "API")
}

func isStructEnum(rt reflect.Type) bool {
	switch rt {
	case reflect.TypeOf(defs.APIOKStatus("")):
		return true

	case reflect.TypeOf(defs.APIErrorStatus("")):
		return true

	case reflect.TypeOf(conf.AuthAction("")):
		return true

	case reflect.TypeOf(conf.AlwaysAvailableTrackCodec("")):
		return true

	case reflect.TypeOf(conf.AuthMethod("")):
		return true

	case reflect.TypeOf(conf.Encryption("")):
		return true

	case reflect.TypeOf(conf.HLSVariant(0)):
		return true

	case reflect.TypeOf(conf.LogDestination(0)):
		return true

	case reflect.TypeOf(conf.LogLevel(0)):
		return true

	case reflect.TypeOf(conf.RecordFormat("")):
		return true

	case reflect.TypeOf(conf.RTSPAuthMethod(0)):
		return true

	case reflect.TypeOf(conf.RTSPRangeType("")):
		return true

	case reflect.TypeOf(conf.RTSPTransport{}):
		return true

	case reflect.TypeOf(defs.APIPathSourceType("")):
		return true

	case reflect.TypeOf(defs.APIPathReaderType("")):
		return true

	case reflect.TypeOf(defs.APIPushTargetProtocol("")):
		return true

	case reflect.TypeOf(defs.APIPushTargetSource("")):
		return true

	case reflect.TypeOf(defs.APIPushTargetState("")):
		return true

	case reflect.TypeOf(defs.APIPathTrackCodec("")):
		return true

	case reflect.TypeOf(defs.APIRTMPConnState("")):
		return true

	case reflect.TypeOf(defs.APIRTSPSessionState("")):
		return true

	case reflect.TypeOf(defs.APIWebRTCSessionState("")):
		return true

	case reflect.TypeOf(defs.APIMoQSessionState("")):
		return true

	case reflect.TypeOf(defs.APISRTConnState("")):
		return true
	}

	return false
}

func extractStructDescriptions(rt reflect.Type) (map[string]string, error) {
	pkgPath := rt.PkgPath()
	dirPath, hasModulePrefix := strings.CutPrefix(pkgPath, modulePathPrefix)
	if !hasModulePrefix {
		return map[string]string{}, nil
	}

	fset := gotoken.NewFileSet()

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		filePath := dirPath + "/" + entry.Name()
		file, parseErr := goparser.ParseFile(fset, filePath, nil, goparser.ParseComments)
		if parseErr != nil {
			return nil, parseErr
		}

		for _, decl := range file.Decls {
			genDecl, genOK := decl.(*goast.GenDecl)
			if !genOK || genDecl.Tok != gotoken.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, typeOK := spec.(*goast.TypeSpec)
				if !typeOK || typeSpec.Name.Name != rt.Name() {
					continue
				}

				structType, structOK := typeSpec.Type.(*goast.StructType)
				if !structOK {
					continue
				}

				descriptions := make(map[string]string)
				for _, field := range structType.Fields.List {
					if field.Tag == nil {
						continue
					}

					jsonTag := extractStructTag(field.Tag.Value, "json")
					jsonName, _, _ := strings.Cut(jsonTag, ",")
					if jsonName == "" || jsonName == "-" {
						continue
					}

					description := ""
					if field.Doc != nil {
						description = normalizeDescription(field.Doc.Text())
					} else if field.Comment != nil {
						description = normalizeDescription(field.Comment.Text())
					}

					if description != "" {
						descriptions[jsonName] = description
					}
				}

				return descriptions, nil
			}
		}
	}

	return map[string]string{}, nil
}

func extractStructTag(tagValue, key string) string {
	tagValue = strings.Trim(tagValue, "`")
	return reflect.StructTag(tagValue).Get(key)
}

func normalizeDescription(description string) string {
	return strings.Join(strings.Fields(description), " ")
}

func generateStructSchema(rt reflect.Type, descriptions map[string]string) (openAPISchema, error) {
	schema := openAPISchema{
		Type:       "object",
		Properties: make(map[string]openAPIProperty),
	}

	for field := range rt.Fields() {
		jsonTag := field.Tag.Get("json")
		name, _, _ := strings.Cut(jsonTag, ",")
		deprecated := field.Tag.Get("deprecated") == "true"

		if name == "" || name == "-" || name == "pathDefaults" || name == "paths" ||
			(strings.Contains(jsonTag, ",omitempty") && !deprecated) {
			continue
		}

		prop, err := goTypeToOpenAPI(field.Type)
		if err != nil {
			return openAPISchema{}, err
		}

		prop.Deprecated = deprecated
		if deprecated {
			prop = wrapRef(field.Type, prop)
		}
		prop.Description = descriptions[name]

		schema.Properties[name] = prop
	}

	return schema, nil
}

func addStructs(astFile *ast.File) error {
	for _, s := range structs {
		descriptions, err := extractStructDescriptions(s.typ)
		if err != nil {
			return err
		}

		schema, err := generateStructSchema(s.typ, descriptions)
		if err != nil {
			return err
		}

		schemaNode, err := yaml.ValueToNode(map[string]openAPISchema{s.externalName: schema})
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
