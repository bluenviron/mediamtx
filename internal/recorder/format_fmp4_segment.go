package recorder

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func writeInit(
	f io.Writer,
	streamID uuid.UUID,
	segmentNumber uint64,
	dts time.Duration,
	ntp time.Time,
	tracks []*formatFMP4Track,
) error {
	fmp4Tracks := make([]*fmp4.InitTrack, len(tracks))
	for i, track := range tracks {
		fmp4Tracks[i] = track.initTrack
	}

	init := fmp4.Init{
		Tracks: fmp4Tracks,
		UserData: []amp4.IBox{
			&recordstore.Mtxi{
				FullBox: amp4.FullBox{
					Version: 0,
				},
				StreamID:      streamID,
				SegmentNumber: segmentNumber,
				DTS:           int64(dts),
				NTP:           ntp.UnixNano(),
			},
		},
	}

	var buf seekablebuffer.Buffer
	err := init.Marshal(&buf)
	if err != nil {
		return err
	}

	_, err = f.Write(buf.Bytes())
	return err
}

func writeDuration(f io.ReadWriteSeeker, d time.Duration) error {
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// check and skip ftyp header and content

	buf := make([]byte, 8)
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return err
	}

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = f.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return err
	}

	// check and skip moov header

	_, err = io.ReadFull(f, buf)
	if err != nil {
		return err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	moovPos, err := f.Seek(8, io.SeekCurrent)
	if err != nil {
		return err
	}

	var mvhd amp4.Mvhd
	_, err = amp4.Unmarshal(f, uint64(moovSize-8), &mvhd, amp4.Context{})
	if err != nil {
		return err
	}

	mvhd.DurationV0 = uint32(d / time.Millisecond)

	_, err = f.Seek(moovPos, io.SeekStart)
	if err != nil {
		return err
	}

	_, err = amp4.Marshal(f, &mvhd, amp4.Context{})
	if err != nil {
		return err
	}

	return nil
}

type formatFMP4Segment struct {
	f        *formatFMP4
	startDTS time.Duration
	startNTP time.Time
	number   uint64

	path           string
	fi             *os.File
	curPart        *formatFMP4Part
	endDTS         time.Duration
	nextPartNumber uint32

	// berry's, CSV sidecar for per-frame timestamps. Each part buffers the
	// rows of its own samples and they are flushed when the part is written.
	csvFi *os.File
}

func (s *formatFMP4Segment) initialize() {
	s.endDTS = s.startDTS
}

func (s *formatFMP4Segment) close() error {
	var err error

	if s.curPart != nil {
		err = s.closeCurPart()
	}

	if s.fi != nil {
		s.f.ri.Log(logger.Debug, "closing segment %s", s.path)

		// write overall duration in the header to speed up the playback server
		duration := s.endDTS - s.startDTS
		err2 := writeDuration(s.fi, duration)
		if err == nil {
			err = err2
		}

		err2 = s.fi.Close()
		if err == nil {
			err = err2
		}

		if err2 == nil {
			s.f.ri.onSegmentComplete(s.path, duration)
		}
	}

	// berry's, close CSV file alongside segment
	if s.csvFi != nil {
		s.csvFi.Close()
	}

	return err
}

func (s *formatFMP4Segment) closeCurPart() error {
	if s.fi == nil {
		s.path = recordstore.Path{Start: s.startNTP}.Encode(s.f.ri.pathFormat2)
		s.f.ri.Log(logger.Debug, "creating segment %s", s.path)

		err := os.MkdirAll(filepath.Dir(s.path), 0o755)
		if err != nil {
			return err
		}

		fi, err := os.Create(s.path)
		if err != nil {
			return err
		}

		s.f.ri.onSegmentCreate(s.path)

		err = writeInit(
			fi,
			s.f.ri.streamID,
			s.number,
			s.startDTS,
			s.startNTP,
			s.f.tracks)
		if err != nil {
			fi.Close()
			return err
		}

		s.fi = fi

		// berry's, the CSV sidecar is created together with the segment, so that
		// it lands in the directory closeCurPart() has just created.
		if s.f.ri.recordTimestampCSV {
			if csvPath, ok := recordstore.CSVPath(s.path); ok {
				csvFi, err2 := os.Create(csvPath)
				if err2 == nil {
					s.csvFi = csvFi
				}
			}
		}
	}

	err := s.curPart.close(s.fi)
	if err != nil {
		return err
	}

	// berry's, flush this part's timestamps only once its samples have reached
	// the segment, so the sidecar never runs ahead of the recording.
	if s.csvFi != nil {
		s.csvFi.Write(s.curPart.csvBuf.Bytes()) //nolint:errcheck
	}

	return nil
}

func (s *formatFMP4Segment) write(track *formatFMP4Track, sample *formatFMP4Sample, dts time.Duration) error {
	endDTS := dts + timestampToDuration(int64(sample.Duration), int(track.initTrack.TimeScale))
	if endDTS > s.endDTS {
		s.endDTS = endDTS
	}

	if s.curPart == nil {
		s.curPart = &formatFMP4Part{
			maxPartSize:     s.f.ri.maxPartSize,
			segmentStartDTS: s.startDTS,
			number:          s.nextPartNumber,
			startDTS:        dts,
		}
		s.curPart.initialize()
		s.nextPartNumber++
	} else if s.curPart.duration() >= s.f.ri.partDuration {
		err := s.closeCurPart()
		s.curPart = nil

		if err != nil {
			return err
		}

		s.curPart = &formatFMP4Part{
			maxPartSize:     s.f.ri.maxPartSize,
			segmentStartDTS: s.startDTS,
			number:          s.nextPartNumber,
			startDTS:        dts,
		}
		s.curPart.initialize()
		s.nextPartNumber++
	}

	err := s.curPart.write(track, sample, dts)
	if err != nil {
		return err
	}

	// berry's, buffer the timestamp of this video sample. It is written to the
	// CSV sidecar by closeCurPart(), together with the part that holds it.
	if s.f.ri.recordTimestampCSV && track.initTrack.Codec.IsVideo() {
		var ts time.Time
		if s.f.ri.recordUseHostTimestamp {
			ts = sample.hostNTP.UTC()
		} else {
			ts = sample.ntp.UTC()
		}
		s.curPart.csvBuf.WriteString(ts.Format("2006-01-02T15:04:05.000000Z,\n"))
	}

	return nil
}
