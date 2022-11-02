package core

import (
	"bytes"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtph264"
)

type streamTrackH264 struct {
	track *gortsplib.TrackH264

	rtpEncoder *rtph264.Encoder
}

func newStreamTrackH264(
	track *gortsplib.TrackH264,
	generateRTPPackets bool,
) *streamTrackH264 {
	t := &streamTrackH264{
		track: track,
	}

	if generateRTPPackets {
		t.rtpEncoder = &rtph264.Encoder{PayloadType: 96}
		t.rtpEncoder.Init()
	}

	return t
}

func (t *streamTrackH264) updateTrackParameters(nalus [][]byte) {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if !bytes.Equal(nalu, t.track.SafeSPS()) {
				t.track.SafeSetSPS(nalu)
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(nalu, t.track.SafePPS()) {
				t.track.SafeSetPPS(nalu)
			}
		}
	}
}

// remux is needed to fix corrupted streams and make streams
// compatible with all protocols.
func (t *streamTrackH264) remuxNALUs(nalus [][]byte) [][]byte {
	addSPSPPS := false
	n := 0
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue
		case h264.NALUTypeAccessUnitDelimiter:
			continue
		case h264.NALUTypeIDR:
			// prepend SPS and PPS to the group if there's at least an IDR
			if !addSPSPPS {
				addSPSPPS = true
				n += 2
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	if addSPSPPS {
		filteredNALUs[0] = t.track.SafeSPS()
		filteredNALUs[1] = t.track.SafePPS()
		i = 2
	}

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			// remove since they're automatically added
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it is not needed
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *streamTrackH264) generateRTPPackets(tdata *dataH264) {
	pkts, err := t.rtpEncoder.Encode(tdata.nalus, tdata.pts)
	if err != nil {
		return
	}

	tdata.rtpPackets = pkts
}

func (t *streamTrackH264) onData(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataH264)

	if tdata.nalus != nil {
		t.updateTrackParameters(tdata.nalus)
		tdata.nalus = t.remuxNALUs(tdata.nalus)
	}

	// NALUS -> RTP
	if t.rtpEncoder != nil {
		t.generateRTPPackets(tdata)
		return nil
	}

	return nil
}
