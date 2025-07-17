package recorder

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func writePart(
	f io.Writer,
	sequenceNumber uint32,
	partTracks map[*formatFMP4Track]*fmp4.PartTrack,
) error {
	fmp4PartTracks := make([]*fmp4.PartTrack, len(partTracks))
	i := 0
	for _, partTrack := range partTracks {
		fmp4PartTracks[i] = partTrack
		i++
	}

	part := &fmp4.Part{
		SequenceNumber: sequenceNumber,
		Tracks:         fmp4PartTracks,
	}

	var buf seekablebuffer.Buffer
	err := part.Marshal(&buf)
	if err != nil {
		return err
	}

	_, err = f.Write(buf.Bytes())
	return err
}

type formatFMP4Part struct {
	s              *formatFMP4Segment
	sequenceNumber uint32
	startDTS       time.Duration

	partTracks map[*formatFMP4Track]*fmp4.PartTrack
	size       uint64
	endDTS     time.Duration
}

func (p *formatFMP4Part) initialize() {
	p.partTracks = make(map[*formatFMP4Track]*fmp4.PartTrack)
}

func (p *formatFMP4Part) close() error {
	if p.s.fi == nil {
		p.s.path = recordstore.Path{Start: p.s.startNTP}.Encode(p.s.f.ri.pathFormat2)
		p.s.f.ri.Log(logger.Debug, "creating segment %s", p.s.path)

		err := os.MkdirAll(filepath.Dir(p.s.path), 0o755)
		if err != nil {
			return err
		}

		fi, err := os.Create(p.s.path)
		if err != nil {
			return err
		}

		p.s.f.ri.onSegmentCreate(p.s.path)

		err = writeInit(fi, p.s.f.tracks)
		if err != nil {
			fi.Close()
			return err
		}

		p.s.fi = fi
	}

	return writePart(p.s.fi, p.sequenceNumber, p.partTracks)
}

func (p *formatFMP4Part) write(track *formatFMP4Track, sample *sample, dts time.Duration) error {
	size := uint64(len(sample.Payload))
	if (p.size + size) > uint64(p.s.f.ri.maxPartSize) {
		return fmt.Errorf("reached maximum part size")
	}
	p.size += size

	partTrack, ok := p.partTracks[track]
	if !ok {
		partTrack = &fmp4.PartTrack{
			ID: track.initTrack.ID,
			BaseTime: uint64(multiplyAndDivide(int64(dts-p.s.startDTS),
				int64(track.initTrack.TimeScale), int64(time.Second))),
		}
		p.partTracks[track] = partTrack
	}

	partTrack.Samples = append(partTrack.Samples, sample.Sample)

	endDTS := dts + timestampToDuration(int64(sample.Duration), int(track.initTrack.TimeScale))
	if endDTS > p.endDTS {
		p.endDTS = endDTS
	}

	return nil
}

func (p *formatFMP4Part) duration() time.Duration {
	return p.endDTS - p.startDTS
}
