package h264

// AntiCompetitionAdd adds the anti-competition bytes to a NALU.
func AntiCompetitionAdd(nalu []byte) []byte {
	var ret []byte
	step := 0
	start := 0

	for i, b := range nalu {
		switch step {
		case 0:
			if b == 0 {
				step++
			}

		case 1:
			if b == 0 {
				step++
			} else {
				step = 0
			}

		case 2:
			switch b {
			case 3, 2, 1, 0:
				ret = append(ret, nalu[start:i-2]...)
				ret = append(ret, []byte{0x00, 0x00, 0x03, b}...)
				step = 0
				start = i + 1

			default:
				step = 0
			}
		}
	}

	ret = append(ret, nalu[start:]...)
	return ret
}

// AntiCompetitionRemove removes the anti-competition bytes from a NALU.
func AntiCompetitionRemove(nalu []byte) []byte {
	// 0x00 0x00 0x03 0x00 -> 0x00 0x00 0x00
	// 0x00 0x00 0x03 0x01 -> 0x00 0x00 0x01
	// 0x00 0x00 0x03 0x02 -> 0x00 0x00 0x02
	// 0x00 0x00 0x03 0x03 -> 0x00 0x00 0x03

	var ret []byte
	step := 0
	start := 0

	for i, b := range nalu {
		switch step {
		case 0:
			if b == 0 {
				step++
			}

		case 1:
			if b == 0 {
				step++
			} else {
				step = 0
			}

		case 2:
			if b == 3 {
				step++
			} else {
				step = 0
			}

		case 3:
			switch b {
			case 3, 2, 1, 0:
				ret = append(ret, nalu[start:i-3]...)
				ret = append(ret, []byte{0x00, 0x00, b}...)
				step = 0
				start = i + 1

			default:
				step = 0
			}
		}
	}

	ret = append(ret, nalu[start:]...)

	return ret
}
