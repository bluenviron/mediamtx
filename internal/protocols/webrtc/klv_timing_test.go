package webrtc

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestVideoTimestampMapper(t *testing.T) {
	var mapper videoTimestampMapper

	_, ok := mapper.translate(90_000)
	require.False(t, ok)

	mapper.update(90_000, 0xfffffff0)

	timestamp, ok := mapper.translate(90_032)
	require.True(t, ok)
	require.Equal(t, uint32(0x10), timestamp)

	timestamp, ok = mapper.translate(89_984)
	require.True(t, ok)
	require.Equal(t, uint32(0xffffffe0), timestamp)
}

func TestMarshalTimedKLV(t *testing.T) {
	klv := unit.PayloadKLV{0x06, 0x0e, 0x2b, 0x34}

	buf := marshalTimedKLV(klv, 0x11223344)

	require.Equal(t, uint8(1), buf[0])
	require.Equal(t, uint8(0), buf[1])
	require.Equal(t, uint16(8), binary.BigEndian.Uint16(buf[2:4]))
	require.Equal(t, uint32(0x11223344), binary.BigEndian.Uint32(buf[4:8]))
	require.Equal(t, []byte{0x06, 0x0e, 0x2b, 0x34}, buf[8:])
}
