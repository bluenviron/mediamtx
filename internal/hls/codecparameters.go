package hls

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/format"
)

func encodeProfileSpace(v uint8) string {
	switch v {
	case 1:
		return "A"
	case 2:
		return "B"
	case 3:
		return "C"
	}
	return ""
}

func encodeCompatibilityFlag(v [32]bool) string {
	var o uint32
	for i, b := range v {
		if b {
			o |= 1 << i
		}
	}
	return fmt.Sprintf("%x", o)
}

func encodeGeneralTierFlag(v uint8) string {
	if v > 0 {
		return "H"
	}
	return "L"
}

func encodeGeneralConstraintIndicatorFlags(v *h265.SPS_ProfileTierLevel) string {
	var ret []string

	var o1 uint8
	if v.GeneralProgressiveSourceFlag {
		o1 |= 1 << 7
	}
	if v.GeneralInterlacedSourceFlag {
		o1 |= 1 << 6
	}
	if v.GeneralNonPackedConstraintFlag {
		o1 |= 1 << 5
	}
	if v.GeneralFrameOnlyConstraintFlag {
		o1 |= 1 << 4
	}
	if v.GeneralMax12bitConstraintFlag {
		o1 |= 1 << 3
	}
	if v.GeneralMax10bitConstraintFlag {
		o1 |= 1 << 2
	}
	if v.GeneralMax8bitConstraintFlag {
		o1 |= 1 << 1
	}
	if v.GeneralMax422ChromeConstraintFlag {
		o1 |= 1 << 0
	}

	ret = append(ret, fmt.Sprintf("%x", o1))

	var o2 uint8
	if v.GeneralMax420ChromaConstraintFlag {
		o2 |= 1 << 7
	}
	if v.GeneralMaxMonochromeConstraintFlag {
		o2 |= 1 << 6
	}
	if v.GeneralIntraConstraintFlag {
		o2 |= 1 << 5
	}
	if v.GeneralOnePictureOnlyConstraintFlag {
		o2 |= 1 << 4
	}
	if v.GeneralLowerBitRateConstraintFlag {
		o2 |= 1 << 3
	}
	if v.GeneralMax14BitConstraintFlag {
		o2 |= 1 << 2
	}

	if o2 != 0 {
		ret = append(ret, fmt.Sprintf("%x", o2))
	}

	return strings.Join(ret, ".")
}

func codecParametersGenerate(track format.Format) string {
	switch ttrack := track.(type) {
	case *format.H264:
		sps := ttrack.SafeSPS()
		if len(sps) >= 4 {
			return "avc1." + hex.EncodeToString(sps[1:4])
		}

	case *format.H265:
		var sps h265.SPS
		err := sps.Unmarshal(ttrack.SafeSPS())
		if err == nil {
			return "hvc1." +
				encodeProfileSpace(sps.ProfileTierLevel.GeneralProfileSpace) +
				strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralProfileIdc), 10) + "." +
				encodeCompatibilityFlag(sps.ProfileTierLevel.GeneralProfileCompatibilityFlag) + "." +
				encodeGeneralTierFlag(sps.ProfileTierLevel.GeneralTierFlag) +
				strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralLevelIdc), 10) + "." +
				encodeGeneralConstraintIndicatorFlags(&sps.ProfileTierLevel)
		}

	case *format.MPEG4Audio:
		// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
		return "mp4a.40." + strconv.FormatInt(int64(ttrack.Config.Type), 10)

	case *format.Opus:
		return "opus"
	}

	return ""
}

func codecParametersAreSupported(codecs string) bool {
	for _, codec := range strings.Split(codecs, ",") {
		if !strings.HasPrefix(codec, "avc1.") &&
			!strings.HasPrefix(codec, "hvc1.") &&
			!strings.HasPrefix(codec, "hev1.") &&
			!strings.HasPrefix(codec, "mp4a.") &&
			codec != "opus" {
			return false
		}
	}
	return true
}
