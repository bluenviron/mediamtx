package core

import (
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpmpeg4audio"
)

type streamTrackMPEG4Audio struct {
	rtpEncoder *rtpmpeg4audio.Encoder
}

func newStreamTrackMPEG4Audio(
	track *gortsplib.TrackMPEG4Audio,
	generateRTPPackets bool,
) *streamTrackMPEG4Audio {
	t := &streamTrackMPEG4Audio{}

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

func (t *streamTrackMPEG4Audio) generateRTPPackets(dat data) []data {
	tdata := dat.(*dataMPEG4Audio)

	pkts, err := t.rtpEncoder.Encode([][]byte{tdata.au}, tdata.pts)
	if err != nil {
		return nil
	}

	ret := make([]data, len(pkts))

	for i, pkt := range pkts {
		ret[i] = &dataMPEG4Audio{
			trackID:   tdata.getTrackID(),
			rtpPacket: pkt,
		}
	}

	return ret
}

func (t *streamTrackMPEG4Audio) process(dat data) []data {
	if dat.getRTPPacket() != nil {
		return []data{dat}
	}

	return t.generateRTPPackets(dat)
}
