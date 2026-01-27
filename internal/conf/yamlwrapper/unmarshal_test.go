package yamlwrapper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmarshalDuplicateKey(t *testing.T) {
	buf := []byte(`
key: value1
key: value2
`)

	err := Unmarshal(buf, &map[string]string{})
	require.EqualError(t, err, "yaml: unmarshal errors:\n  line 3: key \"key\" already set in map")
}
