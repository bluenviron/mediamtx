package stream

import (
	"github.com/bluenviron/mediacommon/v2/pkg/bits"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
)

// Some camera CAVLC slices carry escaped CABAC zero words. Apple decodes
// the IDR but rejects these predictive frames with VideoToolbox -12909.
// H.264 7.4.1.1 permits this padding for CABAC only. Keep CABAC untouched;
// for a matching CAVLC PPS remove padding, exactly as FFmpeg h264_metadata
// does, without changing parameter sets or any compressed picture data.
func h264StripCAVLCPadding(nalu, pps []byte) []byte {
	if len(nalu) < 4 || (nalu[0]&0x1f != 1 && nalu[0]&0x1f != 5) {
		return nalu
	}
	end := len(nalu)
	hasPadding := nalu[end-1] == 0 || (nalu[end-1] == 3 && nalu[end-2] == 0 && nalu[end-3] == 0)
	if !hasPadding {
		return nalu
	}
	if !h264SliceUsesCAVLC(nalu, pps) {
		return nalu
	}
	for end > 1 {
		if nalu[end-1] == 0 ||
			(end >= 4 && nalu[end-1] == 3 && nalu[end-2] == 0 && nalu[end-3] == 0) {
			end--
		} else {
			break
		}
	}
	return nalu[:end]
}

func h264SliceUsesCAVLC(nalu, pps []byte) bool {
	if len(pps) < 2 || pps[0]&0x1f != 8 {
		return false
	}
	parameters := h264.EmulationPreventionRemove(pps)
	pos := 8
	ppsID, err := bits.ReadGolombUnsigned(parameters, &pos)
	if err != nil {
		return false
	}
	if _, err = bits.ReadGolombUnsigned(parameters, &pos); err != nil {
		return false
	}
	cabac, err := bits.ReadFlag(parameters, &pos)
	if err != nil || cabac {
		return false
	}
	// Only the first three Exp-Golomb fields are needed. Avoid copying a
	// potentially multi-megabyte slice to inspect its parameter-set id.
	header := h264.EmulationPreventionRemove(nalu[:min(len(nalu), 64)])
	pos = 8
	var slicePPS uint32
	for range 3 {
		slicePPS, err = bits.ReadGolombUnsigned(header, &pos)
		if err != nil {
			return false
		}
	}
	return slicePPS == ppsID
}
