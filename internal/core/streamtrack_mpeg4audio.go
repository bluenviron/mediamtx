package core

import (
	"fmt"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpcodecs/rtpmpeg4audio"
)

type streamTrackMPEG4Audio struct {
	track   *gortsplib.TrackMPEG4Audio
	encoder *rtpmpeg4audio.Encoder
	decoder *rtpmpeg4audio.Decoder
}

func newStreamTrackMPEG4Audio(
	track *gortsplib.TrackMPEG4Audio,
	allocateEncoder bool,
) *streamTrackMPEG4Audio {
	t := &streamTrackMPEG4Audio{
		track: track,
	}

	if allocateEncoder {
		t.encoder = track.CreateEncoder()
	}

	return t
}

func (t *streamTrackMPEG4Audio) generateRTPPackets(tdata *dataMPEG4Audio) error {
	pkts, err := t.encoder.Encode(tdata.aus, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = pkts
	return nil
}

func (t *streamTrackMPEG4Audio) onData(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataMPEG4Audio)

	if tdata.rtpPackets != nil {
		pkt := tdata.rtpPackets[0]

		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		if pkt.MarshalSize() > maxPacketSize {
			return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
				pkt.MarshalSize(), maxPacketSize)
		}

		// decode from RTP
		if hasNonRTSPReaders {
			if t.decoder == nil {
				t.decoder = t.track.CreateDecoder()
			}

			aus, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg4audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tdata.aus = aus
			tdata.pts = pts
		}

		// route packet as is
		return nil
	}

	return t.generateRTPPackets(tdata)
}
