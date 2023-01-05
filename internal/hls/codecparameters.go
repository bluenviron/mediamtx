package hls

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/format"
)

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
			return "hvc1." + strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralProfileIdc), 10) +
				".4.L" + strconv.FormatInt(int64(sps.ProfileTierLevel.GeneralLevelIdc), 10) + ".B0"
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
			!strings.HasPrefix(codec, "mp4a.") {
			return false
		}
	}
	return true
}
