package hls

import (
	"context"

	"github.com/aler9/gortsplib"
	"github.com/asticode/go-astits"
)

type muxerTSWriter struct {
	innerMuxer     *astits.Muxer
	currentSegment *muxerTSSegment
}

func newMuxerTSWriter(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC) *muxerTSWriter {
	w := &muxerTSWriter{}

	w.innerMuxer = astits.NewMuxer(context.Background(), w)

	if videoTrack != nil {
		w.innerMuxer.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if audioTrack != nil {
		w.innerMuxer.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if videoTrack != nil {
		w.innerMuxer.SetPCRPID(256)
	} else {
		w.innerMuxer.SetPCRPID(257)
	}

	return w
}

func (mt *muxerTSWriter) Write(p []byte) (int, error) {
	return mt.currentSegment.write(p)
}

func (mt *muxerTSWriter) WriteData(d *astits.MuxerData) (int, error) {
	return mt.innerMuxer.WriteData(d)
}
