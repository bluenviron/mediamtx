package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

const (
	templateFile = "internal/apidocsgen/openapi.template.yaml"
	outputFile   = "api/openapi.yaml"
)

type openAPISchema struct {
	Type       string                     `yaml:"type,omitempty"`
	Enum       []string                   `yaml:"enum,omitempty"`
	OneOf      []openAPIProperty          `yaml:"oneOf,omitempty"`
	Properties map[string]openAPIProperty `yaml:"properties,omitempty"`
}

type openAPIProperty struct {
	Ref         string            `yaml:"$ref,omitempty"`
	Type        string            `yaml:"type,omitempty"`
	Format      string            `yaml:"format,omitempty"`
	Description string            `yaml:"description,omitempty"`
	AllOf       []openAPIProperty `yaml:"allOf,omitempty"`
	Nullable    bool              `yaml:"nullable,omitempty"`
	Deprecated  bool              `yaml:"deprecated,omitempty"`
	Enum        []string          `yaml:"enum,omitempty"`
	Items       *openAPIProperty  `yaml:"items,omitempty"`
}

func indentBlockSequences(node ast.Node) {
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, value := range n.Values {
			indentBlockSequences(value)
		}

	case *ast.MappingValueNode:
		if seq, ok := n.Value.(*ast.SequenceNode); ok && !seq.IsFlowStyle && len(seq.Values) > 0 {
			seq.AddColumn(2)
		}
		indentBlockSequences(n.Value)

	case *ast.SequenceNode:
		for _, value := range n.Values {
			indentBlockSequences(value)
		}
	}
}

func addBlankLineBeforeNewSchemaEntry(schemaNode ast.Node) {
	if schemasMapping, ok := schemaNode.(*ast.MappingNode); ok && len(schemasMapping.Values) > 0 {
		keyTk := schemasMapping.Values[0].Key.GetToken()
		keyTk.Position.Line = 3
		keyTk.Prev = &token.Token{
			Type:     token.StringType,
			Position: &token.Position{Line: 1},
		}
	}
}

func parseTemplate() (*ast.File, error) {
	data, err := os.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}

	return parser.ParseBytes(data, parser.ParseComments)
}

func generate() ([]byte, error) {
	astFile, err := parseTemplate()
	if err != nil {
		return nil, err
	}

	err = addEnums(astFile)
	if err != nil {
		return nil, err
	}

	err = addStructs(astFile)
	if err != nil {
		return nil, err
	}

	return []byte(astFile.String()), nil
}

func main() {
	check := flag.Bool("check", false, "check whether the generated OpenAPI matches the file on disk")
	flag.Parse()

	generated, err := generate()
	if err != nil {
		log.Printf("error: %v\n", err)
		os.Exit(1)
	}

	if *check {
		var existing []byte
		existing, err = os.ReadFile(outputFile)
		if err != nil {
			log.Printf("error: %v\n", err)
			os.Exit(1)
		}

		if !bytes.Equal(existing, generated) {
			log.Printf("error: %v\n", fmt.Errorf("%s is outdated, run `go run ./internal/apidocsgen`", outputFile))
			os.Exit(1)
		}

		return
	}

	err = os.WriteFile(outputFile, generated, 0o644)
	if err != nil {
		log.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
