// Package yamlwrapper contains a YAML unmarshaler.
package yamlwrapper

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// differences with respect to the standard package:
// - some legacy YAML 1.1 boolean values (yes, no) are supported
// - all differences of jsonwrapper are inherited

func convertLegacyBools(node ast.Node) ast.Node {
	if node != nil {
		switch n := node.(type) {
		case *ast.MappingNode:
			for _, value := range n.Values {
				convertLegacyBools(value)
			}

		case *ast.MappingValueNode:
			n.Key = convertLegacyBools(n.Key).(ast.MapKeyNode)
			n.Value = convertLegacyBools(n.Value)

		case *ast.SequenceNode:
			for i, value := range n.Values {
				n.Values[i] = convertLegacyBools(value)
			}

		case *ast.DocumentNode:
			n.Body = convertLegacyBools(n.Body)

		case *ast.StringNode:
			if n.Token.Type == token.StringType {
				var boolVal bool
				shouldConvert := false

				switch n.Token.Value {
				case "yes", "Yes", "YES", "on", "On", "ON":
					boolVal = true
					shouldConvert = true

				case "no", "No", "NO", "off", "Off", "OFF":
					boolVal = false
					shouldConvert = true
				}

				if shouldConvert {
					newToken := &token.Token{
						Type:  token.BoolType,
						Value: n.Token.Value,
					}

					if boolVal {
						newToken.Value = "true"
					} else {
						newToken.Value = "false"
					}

					boolNode := ast.Bool(newToken)
					boolNode.Value = boolVal
					return boolNode
				}
			}
		}
	}

	return node
}

// Unmarshal loads the configuration from YAML.
func Unmarshal(buf []byte, dest any) error {
	file, err := parser.ParseBytes(buf, parser.ParseComments)
	if err != nil {
		return err
	}

	if len(file.Docs) != 1 {
		return fmt.Errorf("invalid YAML")
	}

	file.Docs[0] = convertLegacyBools(file.Docs[0]).(*ast.DocumentNode)

	var temp any
	if file.Docs[0].Body != nil {
		err = yaml.NodeToValue(file.Docs[0].Body, &temp)
		if err != nil {
			return err
		}
	}

	// convert the generic map into JSON
	buf, err = json.Marshal(temp)
	if err != nil {
		return err
	}

	// load JSON into destination
	return jsonwrapper.Unmarshal(buf, dest)
}
