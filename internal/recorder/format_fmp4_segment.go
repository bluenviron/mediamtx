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
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/google/uuid"
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

func readBoxHeader(f io.Reader) (string, uint32, error) {
	buf := make([]byte, 8)
	_, err := io.ReadFull(f, buf)
	if err != nil {
		return "", 0, err
	}

	size := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	if size < 8 {
		return "", 0, fmt.Errorf("invalid box size")
	}

	return string(buf[4:]), size, nil
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

	// check and skip ftyp header and content

	typ, ftypSize, err := readBoxHeader(f)
	if err != nil {
		return err
	}

	if typ != "ftyp" {
		return fmt.Errorf("ftyp box not found")
	}

	moovStart, err := f.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return err
	}

	// check moov header

	typ, moovSize, err := readBoxHeader(f)
	if err != nil {
		return err
	}

	if typ != "moov" {
		return fmt.Errorf("moov box not found")
	}

	moovEnd := moovStart + int64(moovSize)

	patchFullBox := func(pos int64, size uint32, box amp4.IBox, patch func()) error {
		_, err2 := f.Seek(pos, io.SeekStart)
		if err2 != nil {
			return err2
		}

		_, err2 = amp4.Unmarshal(f, uint64(size-8), box, amp4.Context{})
		if err2 != nil {
			return err2
		}

		patch()

		_, err2 = f.Seek(pos, io.SeekStart)
		if err2 != nil {
			return err2
		}

		_, err2 = amp4.Marshal(f, box, amp4.Context{})
		return err2
	}

	var movieTimeScale uint32

	// foreach moov child

	for pos := moovStart + 8; pos < moovEnd; {
		_, err = f.Seek(pos, io.SeekStart)
		if err != nil {
			return err
		}

		var size uint32
		typ, size, err = readBoxHeader(f)
		if err != nil {
			return err
		}

		switch typ {
		case "mvhd":
			var mvhd amp4.Mvhd
			err = patchFullBox(pos+8, size, &mvhd, func() {
				movieTimeScale = mvhd.Timescale
				patchDuration(&mvhd,
					func(v uint32) { mvhd.DurationV0 = v },
					func(v uint64) { mvhd.DurationV1 = v },
					d, mvhd.Timescale)
			})
			if err != nil {
				return err
			}

		case "trak":
			trakEnd := pos + int64(size)

			// foreach trak child

			for pos2 := pos + 8; pos2 < trakEnd; {
				_, err = f.Seek(pos2, io.SeekStart)
				if err != nil {
					return err
				}

				var size2 uint32
				typ, size2, err = readBoxHeader(f)
				if err != nil {
					return err
				}

				switch typ {
				case "tkhd":
					var tkhd amp4.Tkhd
					err = patchFullBox(pos2+8, size2, &tkhd, func() {
						patchDuration(&tkhd,
							func(v uint32) { tkhd.DurationV0 = v },
							func(v uint64) { tkhd.DurationV1 = v },
							d, movieTimeScale)
					})
					if err != nil {
						return err
					}

				case "mdia":
					mdiaEnd := pos2 + int64(size2)

					// foreach mdia child

					for pos3 := pos2 + 8; pos3 < mdiaEnd; {
						_, err = f.Seek(pos3, io.SeekStart)
						if err != nil {
							return err
						}

						var size3 uint32
						typ, size3, err = readBoxHeader(f)
						if err != nil {
							return err
						}

						if typ == "mdhd" {
							var mdhd amp4.Mdhd
							err = patchFullBox(pos3+8, size3, &mdhd, func() {
								patchDuration(&mdhd,
									func(v uint32) { mdhd.DurationV0 = v },
									func(v uint64) { mdhd.DurationV1 = v },
									d, mdhd.Timescale)
							})
							if err != nil {
								return err
							}
						}

						pos3 += int64(size3)
					}
				}

				pos2 += int64(size2)
			}
		}

		pos += int64(size)
	}

	return nil
}

type formatFMP4Segment struct {
	f        *formatFMP4
	startDTS time.Duration
	startNTP time.Time
	number   uint64

	path              string
	fi                *os.File
	curPart           *formatFMP4Part
	endDTS            time.Duration
	nextPartNumber    uint32
	seekIndexPos      int64
	seekIndexReserved int
	seekIndexEntries  []seekIndexEntry
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
		err2 := writeSeekIndex(
			s.fi,
			s.seekIndexPos,
			s.seekIndexReserved,
			s.seekIndexEntries,
			duration,
			s.f.tracks)
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
		s.seekIndexReserved = seekIndexReservedEntries(s.f.ri.segmentDuration, s.f.ri.partDuration)
		if s.seekIndexReserved > 0 {
			s.seekIndexPos, err = fi.Seek(0, io.SeekCurrent)
			if err != nil {
				fi.Close()
				return err
			}

			err = writeSeekIndexPlaceholder(fi, len(s.f.tracks), s.seekIndexReserved)
			if err != nil {
				fi.Close()
				return err
			}
		}

		s.fi = fi
	}

	n, err := s.curPart.close(s.fi)
	if err != nil {
		return err
	}

	if s.seekIndexReserved > 0 {
		entryTracks := make([]seekIndexEntryTrack, len(s.f.tracks))
		for i, track := range s.f.tracks {
			if partTrack, ok := s.curPart.partTracks[track]; ok && len(partTrack.Samples) > 0 {
				entryTracks[i] = seekIndexEntryTrack{
					present:  true,
					baseTime: partTrack.BaseTime,
					sap:      !partTrack.Samples[0].IsNonSyncSample,
				}
			}
		}

		s.seekIndexEntries = append(s.seekIndexEntries, seekIndexEntry{
			size:   uint64(n),
			tracks: entryTracks,
		})
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

	return s.curPart.write(track, sample, dts)
}
