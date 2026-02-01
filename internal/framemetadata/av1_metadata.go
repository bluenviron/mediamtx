// Package framemetadata provides utilities to insert and extract per-frame metadata
// from supported codecs (e.g. H264/H265 SEI and AV1 METADATA OBU).
package framemetadata

// buildMetadataOBU builds an AV1 METADATA OBU (obu_type=15) containing payload.
// We always include an OBU size field (LEB128).
func buildMetadataOBU(payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}

	// obu_header:
	// obu_forbidden_bit(1)=0
	// obu_type(4)=15 (metadata)
	// obu_extension_flag(1)=0
	// obu_has_size_field(1)=1
	// obu_reserved_1bit(1)=0
	header := byte(15<<3) | 0x02

	size := encodeLEB128(uint64(len(payload)))
	out := make([]byte, 1+len(size)+len(payload))
	out[0] = header
	copy(out[1:], size)
	copy(out[1+len(size):], payload)
	return out
}

func parseMetadataOBU(obu []byte) ([]byte, bool) {
	if len(obu) < 2 {
		return nil, false
	}
	typ := (obu[0] >> 3) & 0x0F
	if typ != 15 {
		return nil, false
	}
	hasSize := (obu[0] & 0x02) != 0
	if !hasSize {
		// size unknown, treat remaining bytes as payload
		return obu[1:], true
	}

	n, sz, ok := decodeLEB128(obu[1:])
	if !ok {
		return nil, false
	}
	start := 1 + n
	if start+int(sz) > len(obu) {
		return nil, false
	}
	return obu[start : start+int(sz)], true
}

func encodeLEB128(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			break
		}
	}
	return out
}

func decodeLEB128(b []byte) (n int, v uint64, ok bool) {
	var shift uint
	for i := 0; i < len(b) && i < 10; i++ {
		by := b[i]
		v |= uint64(by&0x7F) << shift
		n++
		if (by & 0x80) == 0 {
			return n, v, true
		}
		shift += 7
	}
	return 0, 0, false
}

func frameTypeAV1(tu [][]byte) uint8 {
	// 0 = AV1 key frame
	// 3 = AV1 inter (non-key, non-show-existing)
	//
	// We attempt to parse:
	// - sequence header OBU to read reduced_still_picture_header
	// - first frame header OBU to read show_existing_frame and frame_type
	reducedStill := false
	for _, obu := range tu {
		if len(obu) == 0 {
			continue
		}
		typ := (obu[0] >> 3) & 0x0F
		if typ == 1 { // OBU_SEQUENCE_HEADER
			if v, ok := av1ReducedStillPictureHeader(obu); ok {
				reducedStill = v
			}
		}
	}

	for _, obu := range tu {
		if len(obu) == 0 {
			continue
		}
		typ := (obu[0] >> 3) & 0x0F
		if typ == 3 || typ == 6 { // OBU_FRAME_HEADER or OBU_FRAME
			ft, showExisting, ok := av1FrameHeaderType(obu, reducedStill)
			if ok {
				if showExisting {
					// not explicitly covered by the schema; treat as inter.
					return 3
				}
				if ft == 0 || ft == 2 || ft == 3 { // KEY / INTRA_ONLY / SWITCH
					return 0
				}
				return 3 // INTER
			}
			break
		}
	}

	// fallback
	return 3
}

func av1ReducedStillPictureHeader(obu []byte) (bool, bool) {
	pl, ok := av1OBUPayload(obu)
	if !ok || len(pl) == 0 {
		return false, false
	}
	r := &bitReader{b: pl}
	// seq_profile (3)
	if _, ok2 := r.readBits(3); !ok2 {
		return false, false
	}
	// still_picture (1)
	if _, ok2 := r.readBit(); !ok2 {
		return false, false
	}
	// reduced_still_picture_header (1)
	b, ok2 := r.readBit()
	if !ok2 {
		return false, false
	}
	return b == 1, true
}

func av1FrameHeaderType(obu []byte, reducedStill bool) (frameType uint8, showExisting bool, ok bool) {
	if reducedStill {
		// reduced still picture header implies key frame semantics.
		return 0, false, true
	}

	pl, ok := av1OBUPayload(obu)
	if !ok || len(pl) == 0 {
		return 0, false, false
	}
	r := &bitReader{b: pl}

	// show_existing_frame (1)
	se, ok2 := r.readBit()
	if !ok2 {
		return 0, false, false
	}
	if se == 1 {
		return 0, true, true
	}

	// frame_type (2)
	ft, ok2 := r.readBits(2)
	if !ok2 {
		return 0, false, false
	}
	return uint8(ft), false, true
}

func av1OBUPayload(obu []byte) ([]byte, bool) {
	if len(obu) < 2 {
		return nil, false
	}

	ext := (obu[0] & 0x04) != 0
	hasSize := (obu[0] & 0x02) != 0

	pos := 1
	if ext {
		pos++
		if pos > len(obu) {
			return nil, false
		}
	}

	if !hasSize {
		return obu[pos:], true
	}

	n, sz, ok := decodeLEB128(obu[pos:])
	if !ok {
		return nil, false
	}
	pos += n
	if pos+int(sz) > len(obu) {
		return nil, false
	}
	return obu[pos : pos+int(sz)], true
}

// isMetadataOBUForUs was used by early experiments, but is not part of the current API.
