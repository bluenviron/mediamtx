package recorder

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/google/uuid"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
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

type moofInfo struct {
	offset  uint64
	time    uint64
	trackID uint32
}

func writeMFRA(f io.ReadWriteSeeker, tracks []*formatFMP4Track) error {
	// Get current file size (end of file)
	fileSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	// Seek to start after ftyp and moov
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

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

	_, err = io.ReadFull(f, buf)
	if err != nil {
		return err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	_, err = f.Seek(int64(moovSize)-8, io.SeekCurrent)
	if err != nil {
		return err
	}

	// Collect moof information
	moofInfos := make(map[uint32][]moofInfo) // trackID -> []moofInfo

	for {
		moofPos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}

		_, err = io.ReadFull(f, buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'f'}) {
			break
		}

		moofSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		moofOffset := uint64(moofPos)

		// Parse moof structure directly from file
		moofStartPos := moofPos
		var currentTrackID uint32
		var baseTime uint64
		hasKeyframe := false

		// Seek back to start of moof to parse it
		_, err = f.Seek(moofStartPos, io.SeekStart)
		if err != nil {
			return err
		}

		_, err = amp4.ReadBoxStructure(f, func(h *amp4.ReadHandle) (any, error) {
			switch h.BoxInfo.Type.String() {
			case "moof":
				return h.Expand()

			case "traf":
				return h.Expand()

			case "tfhd":
				box, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				tfhd := box.(*amp4.Tfhd)
				currentTrackID = tfhd.TrackID
				return h.Expand()

			case "tfdt":
				box, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				tfdt := box.(*amp4.Tfdt)
				baseTime = tfdt.BaseMediaDecodeTimeV1
				return nil, nil

			case "trun":
				box, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				trun := box.(*amp4.Trun)
				// Check if any sample is a sync sample (keyframe)
				// SampleFlags: bit 16 is the sync sample flag (0 = sync, 1 = non-sync)
				for _, entry := range trun.Entries {
					if (entry.SampleFlags & 0x00010000) == 0 {
						hasKeyframe = true
						break
					}
				}
				return nil, nil

			case "mdat":
				// Stop parsing when we hit mdat
				return nil, fmt.Errorf("stop")
			}
			return h.Expand()
		})

		// If we found a keyframe in this moof, add it to the list
		if hasKeyframe && currentTrackID != 0 {
			moofInfos[currentTrackID] = append(moofInfos[currentTrackID], moofInfo{
				offset:  moofOffset,
				time:    baseTime,
				trackID: currentTrackID,
			})
		}

		// Seek past the moof and mdat to continue
		_, err = f.Seek(moofStartPos+int64(moofSize), io.SeekStart)
		if err != nil {
			return err
		}

		// Skip mdat
		_, err = io.ReadFull(f, buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'd', 'a', 't'}) {
			break
		}

		mdatSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		_, err = f.Seek(int64(mdatSize)-8, io.SeekCurrent)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	// If no moof boxes found, don't write MFRA
	if len(moofInfos) == 0 {
		return nil
	}

	// Create MFRA box with tfra entries for each track
	tfras := make([]*amp4.Tfra, 0)
	for trackID, infos := range moofInfos {
		if len(infos) == 0 {
			continue
		}

		entries := make([]amp4.TfraEntry, 0, len(infos))
		for _, info := range infos {
			entries = append(entries, amp4.TfraEntry{
				TimeV1:      info.time,
				MoofOffsetV1: info.offset,
				TrafNumber:   1, // traf number within moof (usually 1)
				TrunNumber:   1, // trun number within traf (usually 1)
				SampleNumber: 1, // first sample in trun
			})
		}

		tfra := &amp4.Tfra{
			FullBox: amp4.FullBox{
				Version: 1,
				Flags:   [3]uint8{0, 0, 0},
			},
			TrackID:             trackID,
			Reserved:            0,
			LengthSizeOfTrafNum: 2,
			LengthSizeOfTrunNum: 2,
			LengthSizeOfSampleNum: 2,
			NumberOfEntry:       uint32(len(entries)),
			Entries:             entries,
		}
		tfras = append(tfras, tfra)
	}

	if len(tfras) == 0 {
		return nil
	}

	// Seek to end of file to write MFRA box
	_, err = f.Seek(fileSize, io.SeekStart)
	if err != nil {
		return err
	}

	// Write Mfra container box manually
	// First, write a placeholder for the mfra box size
	mfraStart, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// Write mfra box header placeholder (size will be updated later)
	mfraHeader := make([]byte, 8)
	mfraHeader[4] = 'm'
	mfraHeader[5] = 'f'
	mfraHeader[6] = 'r'
	mfraHeader[7] = 'a'
	_, err = f.Write(mfraHeader)
	if err != nil {
		return err
	}

	// Write all tfra boxes as children of mfra
	for _, tfra := range tfras {
		_, err = amp4.Marshal(f, tfra, amp4.Context{})
		if err != nil {
			return err
		}
	}

	// Update mfra box size
	mfraEnd, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	mfraSize := uint32(mfraEnd - mfraStart)
	_, err = f.Seek(mfraStart, io.SeekStart)
	if err != nil {
		return err
	}
	mfraHeader[0] = byte(mfraSize >> 24)
	mfraHeader[1] = byte(mfraSize >> 16)
	mfraHeader[2] = byte(mfraSize >> 8)
	mfraHeader[3] = byte(mfraSize)
	_, err = f.Write(mfraHeader)
	if err != nil {
		return err
	}

	// Seek back to end to write mfro box
	_, err = f.Seek(mfraEnd, io.SeekStart)
	if err != nil {
		return err
	}

	// Write mfro (Movie Fragment Random Access Offset) box
	// mfro box structure: size(4) + type(4) + version(1) + flags(3) + mfraSize(4) = 16 bytes
	mfroBox := make([]byte, 16)
	mfroBox[0] = 0x00 // size (16 bytes)
	mfroBox[1] = 0x00
	mfroBox[2] = 0x00
	mfroBox[3] = 0x10
	mfroBox[4] = 'm'
	mfroBox[5] = 'f'
	mfroBox[6] = 'r'
	mfroBox[7] = 'o'
	mfroBox[8] = 0x00 // version
	mfroBox[9] = 0x00 // flags[0]
	mfroBox[10] = 0x00 // flags[1]
	mfroBox[11] = 0x00 // flags[2]
	mfroBox[12] = byte(mfraSize >> 24) // mfraSize
	mfroBox[13] = byte(mfraSize >> 16)
	mfroBox[14] = byte(mfraSize >> 8)
	mfroBox[15] = byte(mfraSize)
	_, err = f.Write(mfroBox)
	return err
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

		// write MFRA/MFRO boxes
		err2 = writeMFRA(s.fi, s.f.tracks)
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

		s.fi = fi
	}

	return s.curPart.close(s.fi)
}

func (s *formatFMP4Segment) write(track *formatFMP4Track, sample *sample, dts time.Duration) error {
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
