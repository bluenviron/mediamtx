package h264

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var casesAntiCompetition = []struct {
	name   string
	unproc []byte
	proc   []byte
}{
	{
		"base",
		[]byte{
			0x00, 0x00, 0x00,
			0x00, 0x00, 0x01,
			0x00, 0x00, 0x02,
			0x00, 0x00, 0x03,
		},
		[]byte{
			0x00, 0x00, 0x03, 0x00,
			0x00, 0x00, 0x03, 0x01,
			0x00, 0x00, 0x03, 0x02,
			0x00, 0x00, 0x03, 0x03,
		},
	},
}

func TestAntiCompetitionAdd(t *testing.T) {
	for _, ca := range casesAntiCompetition {
		t.Run(ca.name, func(t *testing.T) {
			proc := AntiCompetitionAdd(ca.unproc)
			require.Equal(t, ca.proc, proc)
		})
	}
}

func TestAntiCompetitionRemove(t *testing.T) {
	for _, ca := range casesAntiCompetition {
		t.Run(ca.name, func(t *testing.T) {
			unproc := AntiCompetitionRemove(ca.proc)
			require.Equal(t, ca.unproc, unproc)
		})
	}
}
