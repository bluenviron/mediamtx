package rtsp

import (
	"math/big"
	"strconv"
	"strings"
)

func normalizeSDPOriginNumericValues(buf []byte) ([]byte, bool) {
	lines := strings.SplitAfter(string(buf), "\n")
	changed := false

	for i, line := range lines {
		lineBody := strings.TrimSuffix(line, "\n")
		lineEnding := line[len(lineBody):]

		if strings.HasSuffix(lineBody, "\r") {
			lineBody = strings.TrimSuffix(lineBody, "\r")
			lineEnding = "\r" + lineEnding
		}

		if !strings.HasPrefix(lineBody, "o=") {
			continue
		}

		fields := strings.Fields(lineBody[2:])
		if len(fields) != 6 {
			continue
		}

		lineChanged := false
		for _, fieldIndex := range []int{1, 2} {
			if _, err := strconv.ParseUint(fields[fieldIndex], 10, 64); err == nil {
				continue
			}

			value, ok := new(big.Int).SetString(fields[fieldIndex], 10)
			if !ok || value.Sign() < 0 {
				continue
			}

			fields[fieldIndex] = strconv.FormatUint(value.Uint64(), 10)
			lineChanged = true
		}

		if lineChanged {
			lines[i] = "o=" + strings.Join(fields, " ") + lineEnding
			changed = true
		}
	}

	if !changed {
		return buf, false
	}

	return []byte(strings.Join(lines, "")), true
}
