package h264

import (
	"encoding/binary"
	"fmt"
)

// DecodeAVCC encodes NALUs from the AVCC code stream format.
func DecodeAVCC(byts []byte) ([][]byte, error) {
	var ret [][]byte

	for len(byts) > 0 {
		if len(byts) < 4 {
			return nil, fmt.Errorf("invalid length")
		}

		le := binary.BigEndian.Uint32(byts)
		byts = byts[4:]

		if len(byts) < int(le) {
			return nil, fmt.Errorf("invalid length")
		}

		ret = append(ret, byts[:le])
		byts = byts[le:]
	}

	if len(ret) == 0 {
		return nil, fmt.Errorf("no NALUs decoded")
	}

	return ret, nil
}

// EncodeAVCC encodes NALUs into the AVCC code stream format.
func EncodeAVCC(nalus [][]byte) ([]byte, error) {
	le := 0
	for _, nalu := range nalus {
		le += 4 + len(nalu)
	}

	ret := make([]byte, le)
	pos := 0

	for _, nalu := range nalus {
		ln := len(nalu)
		binary.BigEndian.PutUint32(ret[pos:], uint32(ln))
		pos += 4

		copy(ret[pos:], nalu)
		pos += ln
	}

	return ret, nil
}
