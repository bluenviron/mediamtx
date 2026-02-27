//go:build enable_linters

package conf

import (
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"github.com/stretchr/testify/require"
)

func checkBooleans(t *testing.T, keys []string, node ast.Node) {
	switch n := node.(type) {
	case *ast.StringNode:
		if n.Token.Type == token.StringType {
			val := strings.ToLower(n.Token.Value)
			if val == "yes" || val == "no" || val == "on" || val == "off" || val == "y" || val == "n" {
				t.Errorf("deprecated bool value '%v: %v'", strings.Join(keys, "."), val)
			}
		}

	case *ast.MappingNode:
		for _, value := range n.Values {
			checkBooleans(t, append(keys, value.Key.(*ast.StringNode).Token.Value), value.Value)
		}
	}
}

func TestConf(t *testing.T) {
	buf, err := os.ReadFile("../../../mediamtx.yml")
	require.NoError(t, err)

	file, err := parser.ParseBytes(buf, 0)
	require.NoError(t, err)

	checkBooleans(t, nil, file.Docs[0].Body)
}
