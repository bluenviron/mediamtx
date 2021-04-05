package h264

import (
	"fmt"
)

// DecodeAnnexB decodes NALUs from the Annex-B code stream format.
func DecodeAnnexB(byts []byte) ([][]byte, error) {
	bl := len(byts)

	// check initial delimiter
	n := func() int {
		if bl < 3 || byts[0] != 0x00 || byts[1] != 0x00 {
			return -1
		}

		if byts[2] == 0x01 {
			return 3
		}

		if bl < 4 || byts[2] != 0x00 || byts[3] != 0x01 {
			return -1
		}

		return 4
	}()
	if n < 0 {
		return nil, fmt.Errorf("input doesn't start with a delimiter")
	}

	var ret [][]byte
	zeros := 0
	start := n
	delimStart := 0

	for i := n; i < bl; i++ {
		switch byts[i] {
		case 0:
			if zeros == 0 {
				delimStart = i
			}
			zeros++

		case 1:
			if zeros == 2 || zeros == 3 {
				nalu := byts[start:delimStart]
				if len(nalu) == 0 {
					return nil, fmt.Errorf("empty NALU")
				}
				ret = append(ret, nalu)
				start = i + 1
			}
			zeros = 0

		default:
			zeros = 0
		}
	}

	nalu := byts[start:bl]
	if len(nalu) == 0 {
		return nil, fmt.Errorf("empty NALU")
	}
	ret = append(ret, nalu)

	return ret, nil
}

// EncodeAnnexB encodes NALUs into the Annex-B code stream format.
func EncodeAnnexB(nalus [][]byte) ([]byte, error) {
	var ret []byte

	for _, nalu := range nalus {
		ret = append(ret, []byte{0x00, 0x00, 0x00, 0x01}...)
		ret = append(ret, nalu...)
	}

	return ret, nil
}
