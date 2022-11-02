package core

import (
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpmpeg4audio"
)

type streamTrackMPEG4Audio struct {
	track      *gortsplib.TrackMPEG4Audio
	rtpEncoder *rtpmpeg4audio.Encoder
	decoder    *rtpmpeg4audio.Decoder
}

func newStreamTrackMPEG4Audio(
	track *gortsplib.TrackMPEG4Audio,
	generateRTPPackets bool,
) *streamTrackMPEG4Audio {
	t := &streamTrackMPEG4Audio{
		track: track,
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

func (t *streamTrackMPEG4Audio) generateRTPPackets(tdata *dataMPEG4Audio) {
	pkts, err := t.rtpEncoder.Encode([][]byte{tdata.aus[0]}, tdata.pts)
	if err != nil {
		return
	}

	tdata.rtpPackets = pkts
}

func (t *streamTrackMPEG4Audio) onData(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataMPEG4Audio)

	// AU -> RTP
	if t.rtpEncoder != nil {
		t.generateRTPPackets(tdata)
		return nil
	}

	// RTP -> AU
	if hasNonRTSPReaders {
		if t.decoder == nil {
			t.decoder = &rtpmpeg4audio.Decoder{
				SampleRate:       t.track.Config.SampleRate,
				SizeLength:       t.track.SizeLength,
				IndexLength:      t.track.IndexLength,
				IndexDeltaLength: t.track.IndexDeltaLength,
			}
			t.decoder.Init()
		}

		aus, pts, err := t.decoder.Decode(tdata.rtpPackets[0])
		if err != nil {
			return err
		}

		tdata.aus = aus
		tdata.pts = pts
	}

	return nil
}
