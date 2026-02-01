package framemetadata

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// buildUserDataUnregisteredSEI builds a SEI NAL unit containing a single
// user_data_unregistered message, with the given UUID and payload.
//
// The payload passed here is the "shared metadata binary format":
// [u16 schemaVersion][16-byte UUID][CBOR payload]
//
// NOTE: user_data_unregistered message syntax mandates a 16-byte UUID field
// at the beginning. We include the payload as-is after that UUID (so the UUID
// is present both as the SEI UUID and inside the shared binary format).
func buildUserDataUnregisteredSEI(isH264 bool, uuid [16]byte, payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}

	// SEI message:
	// payloadType = 5 (user_data_unregistered)
	// payloadSize = 16 (uuid) + len(payload)
	msgPayload := make([]byte, 16+len(payload))
	copy(msgPayload[:16], uuid[:])
	copy(msgPayload[16:], payload)

	msg := make([]byte, 0, 8+len(msgPayload)+1)
	msg = append(msg, encodeSEIValue(5)...)
	msg = append(msg, encodeSEIValue(len(msgPayload))...)
	msg = append(msg, msgPayload...)
	msg = append(msg, 0x80) // rbsp_trailing_bits

	rbsp := applyEmulationPrevention(msg)

	if isH264 {
		// forbidden_zero_bit(1)=0, nal_ref_idc(2)=0, nal_unit_type(5)=6
		out := make([]byte, 1+len(rbsp))
		out[0] = 6
		copy(out[1:], rbsp)
		return out, nil
	}

	// HEVC: NAL header is 2 bytes:
	// forbidden_zero_bit(1)=0
	// nal_unit_type(6)=39 (PREFIX_SEI_NUT)
	// nuh_layer_id(6)=0
	// nuh_temporal_id_plus1(3)=1
	const nalType = 39
	layerID := 0
	tidPlus1 := 1
	h0 := byte((nalType << 1) | ((layerID >> 5) & 0x01))
	h1 := byte(((layerID & 0x1F) << 3) | (tidPlus1 & 0x07))

	out := make([]byte, 2+len(rbsp))
	out[0] = h0
	out[1] = h1
	copy(out[2:], rbsp)
	return out, nil
}

func encodeSEIValue(v int) []byte {
	// SEI payloadType/payloadSize encoding:
	// 0xFF repeated, then remaining (0..254).
	if v < 0 {
		return []byte{0}
	}
	out := make([]byte, 0, v/255+1)
	for v >= 255 {
		out = append(out, 0xFF)
		v -= 255
	}
	out = append(out, byte(v))
	return out
}

// applyEmulationPrevention inserts emulation-prevention bytes (0x03) into RBSP.
func applyEmulationPrevention(rbsp []byte) []byte {
	out := make([]byte, 0, len(rbsp)+len(rbsp)/64)
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 0x03 {
			out = append(out, 0x03)
			zeros = 0
		}
		out = append(out, b)
		if b == 0x00 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// removeEmulationPrevention removes emulation-prevention bytes (0x03) from RBSP.
func removeEmulationPrevention(in []byte) []byte {
	out := make([]byte, 0, len(in))
	zeros := 0
	for i := 0; i < len(in); i++ {
		b := in[i]
		if zeros >= 2 && b == 0x03 {
			// skip EPB
			zeros = 0
			continue
		}
		out = append(out, b)
		if b == 0x00 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func parseUserDataUnregisteredSEI(isH264 bool, nalu []byte) (binaryPayload []byte, ok bool) {
	if isH264 {
		if len(nalu) < 2 || (nalu[0]&0x1F) != 6 {
			return nil, false
		}
		nalu = nalu[1:]
	} else {
		// need 2 bytes header
		if len(nalu) < 3 {
			return nil, false
		}
		nalType := (nalu[0] >> 1) & 0x3F
		if nalType != 39 { // PREFIX_SEI_NUT
			return nil, false
		}
		nalu = nalu[2:]
	}

	rbsp := removeEmulationPrevention(nalu)

	// parse (potentially multiple) SEI messages until rbsp_trailing_bits.
	// we only look for payloadType 5.
	for len(rbsp) > 0 {
		// stop at rbsp_trailing_bits (0x80) if it's alone
		if len(rbsp) == 1 && rbsp[0] == 0x80 {
			break
		}

		pt, n, ok := decodeSEIValue(rbsp)
		if !ok {
			return nil, false
		}
		rbsp = rbsp[n:]

		ps, n, ok := decodeSEIValue(rbsp)
		if !ok {
			return nil, false
		}
		rbsp = rbsp[n:]

		if ps > len(rbsp) {
			return nil, false
		}

		payload := rbsp[:ps]
		rbsp = rbsp[ps:]

		if pt == 5 && len(payload) >= 16 {
			// payload begins with uuid; return the remaining bytes.
			return payload[16:], true
		}
	}

	return nil, false
}

func decodeSEIValue(b []byte) (val int, n int, ok bool) {
	val = 0
	n = 0
	for {
		if n >= len(b) {
			return 0, 0, false
		}
		by := int(b[n])
		n++
		val += by
		if by != 0xFF {
			return val, n, true
		}
	}
}

func insertBeforeVCLH26x(isH264 bool, au [][]byte, sei []byte) [][]byte {
	// Insert after parameter sets / AUD, right before the first VCL NAL.
	idx := -1
	for i, nalu := range au {
		if len(nalu) == 0 {
			continue
		}
		if isVCLH26x(isH264, nalu) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return au
	}

	out := make([][]byte, 0, len(au)+1)
	out = append(out, au[:idx]...)
	out = append(out, sei)
	out = append(out, au[idx:]...)
	return out
}

func isVCLH26x(isH264 bool, nalu []byte) bool {
	if isH264 {
		typ := nalu[0] & 0x1F
		return typ >= 1 && typ <= 5
	}
	if len(nalu) < 2 {
		return false
	}
	typ := (nalu[0] >> 1) & 0x3F
	return typ <= 31
}

func frameTypeH26x(isH264 bool, au [][]byte) uint8 {
	// 0 = I / IDR / CRA
	// 1 = P
	// 2 = B

	// keyframe detection via NAL type
	for _, nalu := range au {
		if len(nalu) == 0 {
			continue
		}
		if isH264 {
			typ := nalu[0] & 0x1F
			if typ == 5 { // IDR
				return 0
			}
		} else {
			if len(nalu) < 2 {
				continue
			}
			typ := (nalu[0] >> 1) & 0x3F
			// IDR_W_RADL(19), IDR_N_LP(20), CRA_NUT(21)
			// also treat BLA (16..18) as keyframe-like.
			if typ == 19 || typ == 20 || typ == 21 || typ == 16 || typ == 17 || typ == 18 {
				return 0
			}
		}
	}

	// non-key: try to parse slice_type from first VCL NAL.
	for _, nalu := range au {
		if len(nalu) == 0 || !isVCLH26x(isH264, nalu) {
			continue
		}
		if isH264 {
			if ft, ok := h264SliceFrameType(nalu); ok {
				return ft
			}
		} else {
			if ft, ok := h265SliceFrameType(nalu); ok {
				return ft
			}
		}
		break
	}

	// fallback
	return 1
}

type bitReader struct {
	b   []byte
	pos int // bit position
}

func (r *bitReader) readBit() (uint8, bool) {
	if r.pos >= len(r.b)*8 {
		return 0, false
	}
	by := r.b[r.pos/8]
	shift := 7 - (r.pos % 8)
	r.pos++
	return (by >> shift) & 0x01, true
}

func (r *bitReader) readBits(n int) (uint64, bool) {
	var v uint64
	for i := 0; i < n; i++ {
		b, ok := r.readBit()
		if !ok {
			return 0, false
		}
		v = (v << 1) | uint64(b)
	}
	return v, true
}

func (r *bitReader) readUE() (uint64, bool) {
	zeros := 0
	for {
		b, ok := r.readBit()
		if !ok {
			return 0, false
		}
		if b == 0 {
			zeros++
			if zeros > 31 {
				return 0, false
			}
			continue
		}
		break
	}
	if zeros == 0 {
		return 0, true
	}
	v, ok := r.readBits(zeros)
	if !ok {
		return 0, false
	}
	return (1 << zeros) - 1 + v, true
}

func h264SliceFrameType(nalu []byte) (uint8, bool) {
	if len(nalu) < 2 {
		return 0, false
	}
	typ := nalu[0] & 0x1F
	if typ != 1 && typ != 5 {
		return 0, false
	}
	rbsp := removeEmulationPrevention(nalu[1:])
	r := &bitReader{b: rbsp}
	// first_mb_in_slice
	if _, ok := r.readUE(); !ok {
		return 0, false
	}
	// slice_type
	st, ok := r.readUE()
	if !ok {
		return 0, false
	}
	st = st % 5
	switch st {
	case 0, 3: // P, SP
		return 1, true
	case 1: // B
		return 2, true
	case 2, 4: // I, SI
		return 0, true
	default:
		return 1, true
	}
}

func h265SliceFrameType(nalu []byte) (uint8, bool) {
	if len(nalu) < 3 {
		return 0, false
	}
	// only parse first_slice_segment_in_pic_flag == 1 case (typical for first VCL in AU).
	rbsp := removeEmulationPrevention(nalu[2:])
	r := &bitReader{b: rbsp}

	first, ok := r.readBit()
	if !ok {
		return 0, false
	}

	nalType := (nalu[0] >> 1) & 0x3F
	if nalType == 19 || nalType == 20 || nalType == 21 || nalType == 16 || nalType == 17 || nalType == 18 {
		if _, ok := r.readBit(); !ok { // no_output_of_prior_pics_flag
			return 0, false
		}
	}

	if _, ok := r.readUE(); !ok { // slice_pic_parameter_set_id
		return 0, false
	}

	if first == 0 {
		// can't reliably skip slice_segment_address without SPS; give up.
		return 0, false
	}

	st, ok := r.readUE() // slice_type: 0=P,1=B,2=I
	if !ok {
		return 0, false
	}

	switch st {
	case 0:
		return 1, true
	case 1:
		return 2, true
	case 2:
		return 0, true
	default:
		return 1, true
	}
}

func mustBinaryHasUUID(in []byte, want [16]byte) bool {
	if len(in) < 2+16 {
		return false
	}
	var got [16]byte
	copy(got[:], in[2:18])
	return bytes.Equal(got[:], want[:])
}

func binarySchemaVersion(in []byte) (uint16, bool) {
	if len(in) < 2 {
		return 0, false
	}
	return binary.BigEndian.Uint16(in[:2]), true
}
