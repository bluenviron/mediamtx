package core

import (
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpmpeg4audio"
)

type streamTrackMPEG4Audio struct {
	writeDataInner func(*data)

	rtpEncoder *rtpmpeg4audio.Encoder
}

func newStreamTrackMPEG4Audio(
	track *gortsplib.TrackMPEG4Audio,
	generateRTPPackets bool,
	writeDataInner func(*data),
) *streamTrackMPEG4Audio {
	t := &streamTrackMPEG4Audio{
		writeDataInner: writeDataInner,
	}

	if generateRTPPackets {
		t.rtpEncoder = &rtpmpeg4audio.Encoder{
			PayloadType:      96,
			SampleRate:       track.ClockRate(),
			SizeLength:       13,
			IndexLength:      3,
			IndexDeltaLength: 3,
		}
		t.rtpEncoder.Init()
	}

	return t
}

func (t *streamTrackMPEG4Audio) generateRTPPackets(dat *data) {
	pkts, err := t.rtpEncoder.Encode([][]byte{dat.mpeg4AudioAU}, dat.pts)
	if err != nil {
		return
	}

	for _, pkt := range pkts {
		t.writeDataInner(&data{
			trackID:      dat.trackID,
			rtpPacket:    pkt,
			ptsEqualsDTS: true,
		})
	}
}

func (t *streamTrackMPEG4Audio) writeData(dat *data) {
	if dat.rtpPacket != nil {
		t.writeDataInner(dat)
	} else {
		t.generateRTPPackets(dat)
	}
}
