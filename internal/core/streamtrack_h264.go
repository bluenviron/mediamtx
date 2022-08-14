package core

import (
	"bytes"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtph264"
)

type streamTrackH264 struct {
	track          *gortsplib.TrackH264
	writeDataInner func(*data)

	rtpEncoder *rtph264.Encoder
}

func newStreamTrackH264(
	track *gortsplib.TrackH264,
	generateRTPPackets bool,
	writeDataInner func(*data),
) *streamTrackH264 {
	t := &streamTrackH264{
		track:          track,
		writeDataInner: writeDataInner,
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

// remux is needed to
// - fix corrupted streams
// - make streams compatible with all protocols
func (t *streamTrackH264) remuxNALUs(nalus [][]byte) [][]byte {
	n := 0
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue
		case h264.NALUTypeAccessUnitDelimiter:
			continue
		case h264.NALUTypeIDR:
			n += 2
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			// remove since they're automatically added before every IDR
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it is not needed
			continue

		case h264.NALUTypeIDR:
			// add SPS and PPS before every IDR
			filteredNALUs[i] = t.track.SafeSPS()
			i++
			filteredNALUs[i] = t.track.SafePPS()
			i++
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *streamTrackH264) generateRTPPackets(dat *data) {
	pkts, err := t.rtpEncoder.Encode(dat.h264NALUs, dat.pts)
	if err != nil {
		return
	}

	lastPkt := len(pkts) - 1
	for i, pkt := range pkts {
		if i != lastPkt {
			t.writeDataInner(&data{
				trackID:   dat.trackID,
				rtpPacket: pkt,
			})
		} else {
			t.writeDataInner(&data{
				trackID:      dat.trackID,
				rtpPacket:    pkt,
				ptsEqualsDTS: dat.ptsEqualsDTS,
				pts:          dat.pts,
				h264NALUs:    dat.h264NALUs,
			})
		}
	}
}

func (t *streamTrackH264) writeData(dat *data) {
	if dat.h264NALUs != nil {
		t.updateTrackParameters(dat.h264NALUs)
		dat.h264NALUs = t.remuxNALUs(dat.h264NALUs)
	}

	if dat.rtpPacket != nil {
		t.writeDataInner(dat)
	} else if dat.h264NALUs != nil {
		t.generateRTPPackets(dat)
	}
}
