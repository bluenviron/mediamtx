package codecprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	mcav1 "github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestAV1RemoveTUD(t *testing.T) {
	forma := &format.AV1{}

	p, err := New(1450, forma, true, nil)
	require.NoError(t, err)

	u := &unit.AV1{
		Base: unit.Base{
			PTS: 30000,
		},
		TU: [][]byte{
			{byte(mcav1.OBUTypeTemporalDelimiter) << 3},
			{5},
		},
	}

	err = p.ProcessUnit(u)
	require.NoError(t, err)

	require.Equal(t, [][]byte{
		{5},
	}, u.TU)
}
