// Package yamlwrapper contains a YAML unmarshaler.
package yamlwrapper

import (
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
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

func countDocumentHeaders(tokens token.Tokens) int {
	documentHeaders := 0
	for _, tk := range tokens {
		if tk.Type == token.DocumentHeaderType {
			documentHeaders++
		}
	}
	return documentHeaders
}

func findDocument(docs []*ast.DocumentNode) (*ast.DocumentNode, error) {
	var doc *ast.DocumentNode

	for _, candidate := range docs {
		if _, ok := candidate.Body.(*ast.DirectiveNode); ok {
			continue
		}

		if doc != nil {
			return nil, fmt.Errorf("invalid YAML")
		}
		doc = candidate
	}

	if doc == nil {
		return nil, fmt.Errorf("invalid YAML")
	}

	return doc, nil
}

// Unmarshal loads the configuration from YAML.
func Unmarshal(buf []byte, dest any) error {
	tokens := lexer.Tokenize(string(buf))

	if countDocumentHeaders(tokens) > 1 {
		return fmt.Errorf("invalid YAML")
	}

	file, err := parser.Parse(tokens, parser.ParseComments)
	if err != nil {
		return err
	}

	doc, err := findDocument(file.Docs)
	if err != nil {
		return err
	}

	doc = convertLegacyBools(doc).(*ast.DocumentNode)

	var temp any
	if doc.Body != nil {
		err = yaml.NodeToValue(doc.Body, &temp)
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
