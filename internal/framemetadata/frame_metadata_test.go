package framemetadata

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestInserterH264KeyThenDelta(t *testing.T) {
	ins := NewInserter()

	// minimal AU containing an IDR slice (NAL type 5)
	au := unit.PayloadH264{
		{0x65, 0x88, 0x80, 0x20}, // not a valid bitstream, but enough for IDR detection
	}

	now := time.UnixMilli(1700000000000)
	clockRate := 90000
	keyPTS := int64(90000 * 10) // 10s
	out := ins.MaybeInsert(&format.H264{}, au, nil, now, time.Time{}, keyPTS, clockRate).(unit.PayloadH264)
	require.Greater(t, len(out), len(au))

	// SEI is inserted before VCL, so first should be SEI (NAL type 6)
	require.Equal(t, byte(6), out[0][0]&0x1F)

	seiPayload, ok := parseUserDataUnregisteredSEI(true, out[0])
	require.True(t, ok)

	ver, id, decoded, err := decodeBinary(seiPayload)
	require.NoError(t, err)
	require.Equal(t, schemaVersion, ver)
	require.Equal(t, uuid16, id)
	require.Equal(t, uint64(0), decoded["frame_type"])
	require.EqualValues(t, int64(10000), decoded["utc_ms"])
	require.EqualValues(t, now.UnixMilli(), decoded["ingest_utc_ms"])

	// second frame (non-IDR slice type 1) => dt_ms
	au2 := unit.PayloadH264{
		{0x41, 0x9a, 0x20, 0x00}, // non-IDR slice, slice header bits are not validated in this test
	}
	now2 := now.Add(33 * time.Millisecond)
	out2 := ins.MaybeInsert(&format.H264{}, au2, nil, now2, time.Time{}, keyPTS+int64(clockRate)*33/1000, clockRate).(unit.PayloadH264)
	require.Equal(t, byte(6), out2[0][0]&0x1F)

	seiPayload2, ok := parseUserDataUnregisteredSEI(true, out2[0])
	require.True(t, ok)

	_, _, decoded2, err := decodeBinary(seiPayload2)
	require.NoError(t, err)
	require.Equal(t, uint64(1), decoded2["frame_type"]) // fallback is P when slice parsing fails
	require.EqualValues(t, int64(int32(33)), decoded2["dt_ms"])
	require.EqualValues(t, int64(int32(33)), decoded2["ingest_dt_ms"])
}

func TestBuildMetadataOBURoundTrip(t *testing.T) {
	blob := []byte{0x00, 0x01, 0x02}
	obu := buildMetadataOBU(blob)
	require.NotNil(t, obu)

	pl, ok := parseMetadataOBU(obu)
	require.True(t, ok)
	require.Equal(t, blob, pl)
}

func TestPTZFieldsOnlyOnVersionChange(t *testing.T) {
	ins := NewInserter()

	pan1 := float32(1.25)
	meta1 := &Meta{PTZ: &PTZ{Ver: 1, Pan: &pan1}}

	au := unit.PayloadH264{
		{0x65, 0x88, 0x80, 0x20}, // IDR
	}
	now := time.UnixMilli(1700000000000)
	clockRate := 90000
	keyPTS := int64(90000 * 10) // 10s
	out := ins.MaybeInsert(&format.H264{}, au, meta1, now, time.Time{}, keyPTS, clockRate).(unit.PayloadH264)
	seiPayload, ok := parseUserDataUnregisteredSEI(true, out[0])
	require.True(t, ok)
	_, _, decoded, err := decodeBinary(seiPayload)
	require.NoError(t, err)
	require.EqualValues(t, uint64(1), decoded["ptz_ver"])
	require.EqualValues(t, float32(1.25), decoded["pan"])

	// Same version, different pan -> should be omitted.
	pan2 := float32(2.5)
	meta2 := &Meta{PTZ: &PTZ{Ver: 1, Pan: &pan2}}
	out2 := ins.MaybeInsert(&format.H264{}, au, meta2, now.Add(40*time.Millisecond), time.Time{}, keyPTS+int64(clockRate)*40/1000, clockRate).(unit.PayloadH264)
	seiPayload2, ok := parseUserDataUnregisteredSEI(true, out2[0])
	require.True(t, ok)
	_, _, decoded2, err := decodeBinary(seiPayload2)
	require.NoError(t, err)
	_, hasVer := decoded2["ptz_ver"]
	_, hasPan := decoded2["pan"]
	require.False(t, hasVer)
	require.False(t, hasPan)

	// Version changes -> emitted again.
	meta3 := &Meta{PTZ: &PTZ{Ver: 2, Pan: &pan2}}
	out3 := ins.MaybeInsert(&format.H264{}, au, meta3, now.Add(80*time.Millisecond), time.Time{}, keyPTS+int64(clockRate)*80/1000, clockRate).(unit.PayloadH264)
	seiPayload3, ok := parseUserDataUnregisteredSEI(true, out3[0])
	require.True(t, ok)
	_, _, decoded3, err := decodeBinary(seiPayload3)
	require.NoError(t, err)
	require.EqualValues(t, uint64(2), decoded3["ptz_ver"])
	require.EqualValues(t, float32(2.5), decoded3["pan"])
}

