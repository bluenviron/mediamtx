package framemetadata

import (
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/ugorji/go/codec"

	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	schemaVersion uint16 = 1
)

// fixed UUID for this feature (stable across frames).
var uuid16 = [16]byte{
	0x1d, 0x53, 0x6d, 0x9a, 0x2b, 0x2d, 0x44, 0x18,
	0x93, 0x3e, 0x2a, 0x3c, 0xf8, 0x0f, 0x60, 0x5c,
}

type inserterState struct {
	lastKeyCamMS    int64
	lastKeyIngestMS int64
	lastKeyPTS      int64
	lastPTZVer      uint32
}

// PTZ contains optional PTZ metadata.
// Fields are emitted only when Ver changes.
type PTZ struct {
	Ver  uint32
	Pan  *float32
	Tilt *float32
	Zoom *float32
}

// Meta contains optional per-frame metadata.
type Meta struct {
	PTZ *PTZ
}

// Inserter injects per-frame metadata into codec access units / temporal units.
// It is stateful (dt_ms and ptz_ver change suppression).
type Inserter struct {
	st inserterState
}

// NewInserter allocates a new Inserter.
func NewInserter() *Inserter {
	return &Inserter{}
}

// MaybeInsert inserts metadata into the provided payload (if supported by the codec),
// and returns the new payload. The caller passes "now" as the UTC time source.
//
// This intentionally operates on the internal representation:
// - H264/H265: unit.PayloadH264 / unit.PayloadH265 ([][]byte NALUs without start codes)
// - AV1: unit.PayloadAV1 ([][]byte OBUs)
func (i *Inserter) MaybeInsert(
	forma format.Format,
	payload unit.Payload,
	meta *Meta,
	ingestNow time.Time,
	ntp time.Time,
	pts int64,
	clockRate int,
) unit.Payload {
	switch forma.(type) {
	case *format.H264:
		au, ok := payload.(unit.PayloadH264)
		if !ok || len(au) == 0 {
			return payload
		}
		return unit.PayloadH264(i.insertH26x(true, au, meta, ingestNow, ntp, pts, clockRate))

	case *format.H265:
		au, ok := payload.(unit.PayloadH265)
		if !ok || len(au) == 0 {
			return payload
		}
		return unit.PayloadH265(i.insertH26x(false, au, meta, ingestNow, ntp, pts, clockRate))

	case *format.AV1:
		tu, ok := payload.(unit.PayloadAV1)
		if !ok || len(tu) == 0 {
			return payload
		}
		return unit.PayloadAV1(i.insertAV1(tu, meta, ingestNow, ntp, pts, clockRate))
	}

	return payload
}

func (i *Inserter) insertH26x(
	isH264 bool,
	au [][]byte,
	meta *Meta,
	ingestNow time.Time,
	ntp time.Time,
	pts int64,
	clockRate int,
) [][]byte {
	ft := frameTypeH26x(isH264, au)
	blob, err := i.buildBinary(ft, meta, ingestNow, ntp, pts, clockRate)
	if err != nil {
		return au
	}

	sei, err := buildUserDataUnregisteredSEI(isH264, uuid16, blob)
	if err != nil {
		return au
	}

	return insertBeforeVCLH26x(isH264, au, sei)
}

func (i *Inserter) insertAV1(
	tu [][]byte,
	meta *Meta,
	ingestNow time.Time,
	ntp time.Time,
	pts int64,
	clockRate int,
) [][]byte {
	ft := frameTypeAV1(tu)
	blob, err := i.buildBinary(ft, meta, ingestNow, ntp, pts, clockRate)
	if err != nil {
		return tu
	}

	obu := buildMetadataOBU(blob)
	if obu == nil {
		return tu
	}

	out := make([][]byte, 0, len(tu)+1)
	out = append(out, obu)
	out = append(out, tu...)
	return out
}

func (i *Inserter) buildBinary(
	frameType uint8,
	meta *Meta,
	ingestNow time.Time,
	ntp time.Time,
	pts int64,
	clockRate int,
) ([]byte, error) {
	payloadCBOR, err := i.buildCBORPayload(frameType, meta, ingestNow, ntp, pts, clockRate)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 2+16+len(payloadCBOR))
	binary.BigEndian.PutUint16(out[:2], schemaVersion)
	copy(out[2:18], uuid16[:])
	copy(out[18:], payloadCBOR)
	return out, nil
}

func (i *Inserter) buildCBORPayload(
	frameType uint8,
	meta *Meta,
	ingestNow time.Time,
	ntp time.Time,
	pts int64,
	clockRate int,
) ([]byte, error) {
	// determine utc_ms / dt_ms (camera clock) and ingest_utc_ms / ingest_dt_ms (server clock)
	m := make(map[string]any, 6)
	m["frame_type"] = uint64(frameType)
	m["version"] = "v0.1"

	if frameType == 0 {
		// Camera absolute time:
		// - prefer RTCP/SR-derived NTP if available (can differ from server clock)
		// - otherwise fall back to the stream clock converted to ms.
		var camKeyMS int64
		if !ntp.IsZero() {
			camKeyMS = ntp.UnixMilli()
		} else {
			camKeyMS = ptsToMillis(pts, clockRate)
		}

		ingestKeyMS := ingestNow.UnixMilli()

		m["utc_ms"] = camKeyMS
		m["ingest_utc_ms"] = ingestKeyMS

		i.st.lastKeyCamMS = camKeyMS
		i.st.lastKeyIngestMS = ingestKeyMS
		i.st.lastKeyPTS = pts
	} else {
		// Both dt_ms and ingest_dt_ms advance according to the stream (PTS),
		// not wall-clock time, to avoid jitter due to network/processing delays.
		dt := int64(0)
		if i.st.lastKeyPTS != 0 {
			dt = ptsToMillis(pts-i.st.lastKeyPTS, clockRate)
		}
		if dt < -2147483648 || dt > 2147483647 {
			return nil, errors.New("dt_ms overflows int32")
		}
		m["dt_ms"] = int64(int32(dt))
		m["ingest_dt_ms"] = int64(int32(dt))
	}

	// PTZ fields are included only when ptz_ver changes.
	if meta != nil && meta.PTZ != nil {
		if meta.PTZ.Ver != i.st.lastPTZVer {
			i.st.lastPTZVer = meta.PTZ.Ver
			m["ptz_ver"] = uint64(meta.PTZ.Ver)
			if meta.PTZ.Pan != nil {
				m["pan"] = *meta.PTZ.Pan
			}
			if meta.PTZ.Tilt != nil {
				m["tilt"] = *meta.PTZ.Tilt
			}
			if meta.PTZ.Zoom != nil {
				m["zoom"] = *meta.PTZ.Zoom
			}
		}
	}

	var mh codec.CborHandle
	mh.Canonical = true
	var buf []byte
	enc := codec.NewEncoderBytes(&buf, &mh)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf, nil
}

func ptsToMillis(pts int64, clockRate int) int64 {
	if clockRate <= 0 {
		return 0
	}
	// round to nearest millisecond
	// ms = pts * 1000 / clockRate
	num := pts * 1000
	if num >= 0 {
		return (num + int64(clockRate)/2) / int64(clockRate)
	}
	return (num - int64(clockRate)/2) / int64(clockRate)
}

func decodeBinary(in []byte) (uint16, [16]byte, map[string]any, error) {
	if len(in) < 2+16 {
		return 0, [16]byte{}, nil, errors.New("buffer too short")
	}
	ver := binary.BigEndian.Uint16(in[:2])
	var id [16]byte
	copy(id[:], in[2:18])
	cborPayload := in[18:]

	var mh codec.CborHandle
	dec := codec.NewDecoderBytes(cborPayload, &mh)

	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return 0, [16]byte{}, nil, err
	}
	return ver, id, out, nil
}

// HookConfig configures MaybeInsertFrameMetadata().
// This is intentionally minimal: callers can keep a per-stream State to preserve dt_ms logic.
type HookConfig struct {
	Enabled bool
	State   *Inserter
}

// HookMeta is reserved for future (PTZ, etc). It's optional.
type HookMeta struct{}

// MaybeInsertFrameMetadata is the single external hook.
//
// - If disabled, it returns frame unchanged.
// - For "h264"/"h265", frame must be Annex-B (start-code delimited).
// - For "av1", frame must be an OBU stream where each OBU has a size field.
//
// NOTE: MediaMTX internally does not use this function directly (it operates on
// unit.Payload*), but it is provided as the requested integration hook.
func MaybeInsertFrameMetadata(codecName string, frame []byte, _ *HookMeta, cfg *HookConfig) []byte {
	if cfg == nil || !cfg.Enabled || len(frame) == 0 {
		return frame
	}
	if cfg.State == nil {
		cfg.State = NewInserter()
	}

	switch strings.ToLower(codecName) {
	case "h264":
		au := splitAnnexB(frame)
		if len(au) == 0 {
			return frame
		}
		au2 := cfg.State.insertH26x(true, au, nil, time.Now(), time.Time{}, 0, 0)
		return joinAnnexB(au2)

	case "h265", "hevc":
		au := splitAnnexB(frame)
		if len(au) == 0 {
			return frame
		}
		au2 := cfg.State.insertH26x(false, au, nil, time.Now(), time.Time{}, 0, 0)
		return joinAnnexB(au2)

	case "av1":
		tu := splitOBUStream(frame)
		if len(tu) == 0 {
			return frame
		}
		tu2 := cfg.State.insertAV1(tu, nil, time.Now(), time.Time{}, 0, 0)
		return joinOBUStream(tu2)

	default:
		return frame
	}
}

func splitAnnexB(b []byte) [][]byte {
	// Parses both 3-byte and 4-byte start codes, returns NALUs without start codes.
	var out [][]byte
	i := 0
	for {
		scPos, scLen := findStartCode(b, i)
		if scPos < 0 {
			break
		}
		j := scPos + scLen
		nextPos, _ := findStartCode(b, j)
		if nextPos < 0 {
			nextPos = len(b)
		}
		if j < nextPos {
			nalu := bytesClone(b[j:nextPos])
			if len(nalu) > 0 {
				out = append(out, nalu)
			}
		}
		i = nextPos
	}
	return out
}

func joinAnnexB(nalus [][]byte) []byte {
	if len(nalus) == 0 {
		return nil
	}
	// Use 4-byte start codes.
	total := 0
	for _, n := range nalus {
		total += 4 + len(n)
	}
	out := make([]byte, 0, total)
	for _, n := range nalus {
		out = append(out, 0x00, 0x00, 0x00, 0x01)
		out = append(out, n...)
	}
	return out
}

func findStartCode(b []byte, start int) (pos int, length int) {
	for i := start; i+3 <= len(b); i++ {
		if i+4 <= len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 1 {
			return i, 4
		}
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			return i, 3
		}
	}
	return -1, 0
}

func splitOBUStream(b []byte) [][]byte {
	// Assumes each OBU has a size field and no extension.
	// This is a best-effort splitter for the external hook.
	var out [][]byte
	for i := 0; i < len(b); {
		if i+2 > len(b) {
			break
		}
		h := b[i]
		ext := (h & 0x04) != 0
		hasSize := (h & 0x02) != 0
		j := i + 1
		if ext {
			j++
			if j > len(b) {
				break
			}
		}
		if !hasSize {
			// can't split safely; return none to avoid corruption
			return nil
		}
		n, sz, ok := decodeLEB128(b[j:])
		if !ok {
			return nil
		}
		j += n
		if j+int(sz) > len(b) {
			return nil
		}
		obu := bytesClone(b[i : j+int(sz)])
		out = append(out, obu)
		i = j + int(sz)
	}
	return out
}

func joinOBUStream(obus [][]byte) []byte {
	total := 0
	for _, o := range obus {
		total += len(o)
	}
	out := make([]byte, 0, total)
	for _, o := range obus {
		out = append(out, o...)
	}
	return out
}

func bytesClone(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
