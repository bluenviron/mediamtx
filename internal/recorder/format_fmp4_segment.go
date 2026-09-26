package recorder

import (
	"fmt"
	"io"
	"math"
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

func scaleDuration(d time.Duration, timeScale uint32) uint64 {
	return uint64(multiplyAndDivide(int64(d), int64(timeScale), int64(time.Second)))
}

func patchDuration(box interface {
	GetVersion() uint8
}, setV0 func(uint32), setV1 func(uint64), d time.Duration, timeScale uint32,
) {
	v := scaleDuration(d, timeScale)
	if box.GetVersion() == 1 {
		setV1(v)
	} else {
		if v > math.MaxUint32 {
			v = math.MaxUint32
		}
		setV0(uint32(v))
	}
}

// writeDuration writes the overall duration into the mvhd, tkhd and mdhd
// boxes of the header, to speed up the playback server and to allow
// external players to obtain the duration without reading the whole file.
func writeDuration(f io.ReadWriteSeeker, d time.Duration) error {
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// collect the position of every box to patch; patching is done
	// afterwards in order not to write to the file while it's being walked.
	var mvhdInfo *amp4.BoxInfo
	var tkhdInfos []*amp4.BoxInfo
	var mdhdInfos []*amp4.BoxInfo

	_, err = amp4.ReadBoxStructure(f, func(h *amp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type.String() {
		case "moov", "trak", "mdia":
			return h.Expand()

		case "mvhd":
			bi := h.BoxInfo
			mvhdInfo = &bi

		case "tkhd":
			bi := h.BoxInfo
			tkhdInfos = append(tkhdInfos, &bi)

		case "mdhd":
			bi := h.BoxInfo
			mdhdInfos = append(mdhdInfos, &bi)
		}
		return nil, nil
	})
	if err != nil {
		return err
	}

	if mvhdInfo == nil {
		return fmt.Errorf("mvhd box not found")
	}

	patchFullBox := func(bi *amp4.BoxInfo, box amp4.IBox, patch func()) error {
		payloadPos := int64(bi.Offset + bi.HeaderSize)

		_, err2 := f.Seek(payloadPos, io.SeekStart)
		if err2 != nil {
			return err2
		}

		_, err2 = amp4.Unmarshal(f, bi.Size-bi.HeaderSize, box, amp4.Context{})
		if err2 != nil {
			return err2
		}

		patch()

		_, err2 = f.Seek(payloadPos, io.SeekStart)
		if err2 != nil {
			return err2
		}

		_, err2 = amp4.Marshal(f, box, amp4.Context{})
		return err2
	}

	// patch mvhd first: tkhd durations are expressed in the movie timescale.
	var movieTimeScale uint32

	var mvhd amp4.Mvhd
	err = patchFullBox(mvhdInfo, &mvhd, func() {
		movieTimeScale = mvhd.Timescale
		patchDuration(&mvhd,
			func(v uint32) { mvhd.DurationV0 = v },
			func(v uint64) { mvhd.DurationV1 = v },
			d, mvhd.Timescale)
	})
	if err != nil {
		return err
	}

	for _, bi := range tkhdInfos {
		var tkhd amp4.Tkhd
		err = patchFullBox(bi, &tkhd, func() {
			patchDuration(&tkhd,
				func(v uint32) { tkhd.DurationV0 = v },
				func(v uint64) { tkhd.DurationV1 = v },
				d, movieTimeScale)
		})
		if err != nil {
			return err
		}
	}

	for _, bi := range mdhdInfos {
		var mdhd amp4.Mdhd
		err = patchFullBox(bi, &mdhd, func() {
			patchDuration(&mdhd,
				func(v uint32) { mdhd.DurationV0 = v },
				func(v uint64) { mdhd.DurationV1 = v },
				d, mdhd.Timescale)
		})
		if err != nil {
			return err
		}
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
	seekIndex      seekIndex
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

		duration := s.endDTS - s.startDTS

		// write a seek index to allow players to seek without reading the whole file
		err2 := s.seekIndex.write(s.fi, duration, s.f.tracks)
		if err == nil {
			err = err2
		}

		// write overall duration in the header to speed up the playback server
		err2 = writeDuration(s.fi, duration)
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

		// reserve space for the seek index
		err = s.seekIndex.reserve(fi, s.f.ri.segmentDuration, s.f.ri.partDuration, len(s.f.tracks))
		if err != nil {
			fi.Close()
			return err
		}

		s.fi = fi
	}

	n, err := s.curPart.close(s.fi)
	if err != nil {
		return err
	}

	s.seekIndex.recordPart(n, s.curPart.partTracks, s.f.tracks)

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

	return s.curPart.write(track, sample, dts)
}
