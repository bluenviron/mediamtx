package framemetadata

import "fmt"

// Decoded is a decoded metadata message.
type Decoded struct {
	SchemaVersion uint16
	UUID          [16]byte
	CBOR          map[string]any
}

// ExtractFromH264AU extracts metadata from a H.264 access unit (NALUs without start codes).
func ExtractFromH264AU(au [][]byte) (*Decoded, bool, error) {
	return extractFromH26xAU(true, au)
}

// ExtractFromH265AU extracts metadata from a H.265 access unit (NALUs without start codes).
func ExtractFromH265AU(au [][]byte) (*Decoded, bool, error) {
	return extractFromH26xAU(false, au)
}

func extractFromH26xAU(isH264 bool, au [][]byte) (*Decoded, bool, error) {
	for _, nalu := range au {
		raw, ok := parseUserDataUnregisteredSEI(isH264, nalu)
		if !ok {
			continue
		}

		ver, id, cbor, err := decodeBinary(raw)
		if err != nil {
			return nil, false, err
		}
		if ver != schemaVersion || id != uuid16 {
			continue
		}

		return &Decoded{
			SchemaVersion: ver,
			UUID:          id,
			CBOR:          cbor,
		}, true, nil
	}

	return nil, false, nil
}

// ExtractFromAV1TU extracts metadata from an AV1 temporal unit (OBUs).
func ExtractFromAV1TU(tu [][]byte) (*Decoded, bool, error) {
	for _, obu := range tu {
		pl, ok := parseMetadataOBU(obu)
		if !ok {
			continue
		}

		ver, id, cbor, err := decodeBinary(pl)
		if err != nil {
			return nil, false, err
		}
		if ver != schemaVersion || id != uuid16 {
			continue
		}

		return &Decoded{
			SchemaVersion: ver,
			UUID:          id,
			CBOR:          cbor,
		}, true, nil
	}

	return nil, false, nil
}

func asUint64(m map[string]any, k string) (uint64, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case uint64:
		return x, true
	case uint32:
		return uint64(x), true
	case uint16:
		return uint64(x), true
	case uint8:
		return uint64(x), true
	case int64:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case int:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	default:
		return 0, false
	}
}

func asInt64(m map[string]any, k string) (int64, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int64:
		return x, true
	case int32:
		return int64(x), true
	case int:
		return int64(x), true
	case uint64:
		return int64(x), true
	case uint32:
		return int64(x), true
	default:
		return 0, false
	}
}

func asFloat32(m map[string]any, k string) (float32, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float32:
		return x, true
	case float64:
		return float32(x), true
	default:
		return 0, false
	}
}

// FormatOverlayText converts a decoded CBOR payload into a human-readable overlay string.
func FormatOverlayText(d *Decoded) string {
	if d == nil || d.CBOR == nil {
		return ""
	}
	m := d.CBOR

	ft, _ := asUint64(m, "frame_type")

	if ft == 0 {
		cam, _ := asInt64(m, "utc_ms")
		ing, _ := asInt64(m, "ingest_utc_ms")
		s := fmt.Sprintf("key  cam_utc_ms=%d  ingest_utc_ms=%d", cam, ing)
		if v, ok := asUint64(m, "ptz_ver"); ok {
			s += fmt.Sprintf("  ptz_ver=%d", v)
			if pan, ok := asFloat32(m, "pan"); ok {
				s += fmt.Sprintf("  pan=%.3f", pan)
			}
			if tilt, ok := asFloat32(m, "tilt"); ok {
				s += fmt.Sprintf("  tilt=%.3f", tilt)
			}
			if zoom, ok := asFloat32(m, "zoom"); ok {
				s += fmt.Sprintf("  zoom=%.3f", zoom)
			}
		}
		return s
	}

	dt, _ := asInt64(m, "dt_ms")
	idt, _ := asInt64(m, "ingest_dt_ms")
	return fmt.Sprintf("nonkey  cam_dt_ms=%d  ingest_dt_ms=%d  frame_type=%d", dt, idt, ft)
}
