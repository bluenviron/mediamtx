package recorder

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"slices"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

const (
	// reference_count in sidx is 16 bits.
	seekIndexMaxEntries = 65535

	// size of a version-1 sidx box with zero entries.
	seekIndexFixedSize = 40

	// size of each sidx reference.
	seekIndexEntrySize = 12
)

// seekIndexEntryTrack describes the samples of a single track inside a part.
type seekIndexEntryTrack struct {
	present  bool
	baseTime uint64 // DTS of the first sample, in track timescale, relative to segment start
	sap      bool   // whether the first sample is a sync sample
}

// seekIndexEntry describes a part (moof + mdat pair) of a segment.
type seekIndexEntry struct {
	size   uint64
	tracks []seekIndexEntryTrack // aligned with formatFMP4.tracks
}

// seekIndexBoxSize returns the size of a sidx box with the given number of
// references.
func seekIndexBoxSize(entries int) int {
	return seekIndexFixedSize + entries*seekIndexEntrySize
}

// seekIndexReservedEntries returns the number of sidx references to reserve
// space for. Parts are at least partDuration long, therefore a segment
// normally consists of at most (segmentDuration / partDuration + 1) parts;
// one more is added since a segment closes at the first random access sample
// after segmentDuration. If a segment turns out to contain even more parts,
// entries are coalesced.
func seekIndexReservedEntries(segmentDuration time.Duration, partDuration time.Duration) int {
	if partDuration <= 0 {
		return 0
	}
	return min(int(segmentDuration/partDuration)+2, seekIndexMaxEntries)
}

// writeSeekIndexPlaceholder writes a free box that reserves the space needed
// by one sidx box per track with the given number of references.
func writeSeekIndexPlaceholder(w io.Writer, trackCount int, entries int) error {
	buf := make([]byte, trackCount*seekIndexBoxSize(entries))
	binary.BigEndian.PutUint32(buf, uint32(len(buf)))
	copy(buf[4:8], "free")
	_, err := w.Write(buf)
	return err
}

// mergeSeekIndexEntries merges src into dst, which must precede it.
func mergeSeekIndexEntries(dst *seekIndexEntry, src seekIndexEntry) {
	dst.size += src.size
	for i := range dst.tracks {
		if !dst.tracks[i].present {
			dst.tracks[i] = src.tracks[i]
		}
	}
}

// seekIndex accumulates the layout of the parts written into a segment and
// turns it into one sidx box per track when the segment closes. Space for
// the index is reserved with a free box when the segment file is created;
// recordings interrupted by a crash keep the placeholder and remain
// readable.
type seekIndex struct {
	pos      int64
	reserved int
	entries  []seekIndexEntry
}

// reserve writes a placeholder free box at the current file position,
// sized for the number of references the segment is expected to need.
func (si *seekIndex) reserve(
	f io.WriteSeeker,
	segmentDuration time.Duration,
	partDuration time.Duration,
	trackCount int,
) error {
	si.reserved = seekIndexReservedEntries(segmentDuration, partDuration)
	if si.reserved <= 0 {
		return nil
	}

	var err error
	si.pos, err = f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	return writeSeekIndexPlaceholder(f, trackCount, si.reserved)
}

// recordPart records the size and per-track layout of a part
// (moof + mdat pair) that has just been written.
func (si *seekIndex) recordPart(
	size int,
	partTracks map[*formatFMP4Track]*fmp4.PartTrack,
	tracks []*formatFMP4Track,
) {
	if si.reserved <= 0 {
		return
	}

	entryTracks := make([]seekIndexEntryTrack, len(tracks))
	for i, track := range tracks {
		if partTrack, ok := partTracks[track]; ok && len(partTrack.Samples) > 0 {
			entryTracks[i] = seekIndexEntryTrack{
				present:  true,
				baseTime: partTrack.BaseTime,
				sap:      !partTrack.Samples[0].IsNonSyncSample,
			}
		}
	}

	si.entries = append(si.entries, seekIndexEntry{
		size:   uint64(size),
		tracks: entryTracks,
	})

	// the index can never reference more than reserved parts, and write()
	// coalesces entries to fit, so granularity beyond that is discarded
	// anyway. Merge adjacent pairs once entries reach twice the reserved
	// count, to keep memory usage bounded when partDuration is much smaller
	// than segmentDuration.
	if len(si.entries) >= 2*si.reserved {
		for i := 0; i < len(si.entries); i += 2 {
			e := si.entries[i]
			if i+1 < len(si.entries) {
				mergeSeekIndexEntries(&e, si.entries[i+1])
			}
			si.entries[i/2] = e
		}
		si.entries = si.entries[:(len(si.entries)+1)/2]
	}
}

// write overwrites the placeholder written by reserve with one sidx box per
// track, allowing players to seek without scanning the whole file. A sidx
// per track is needed since FFmpeg-based players seek through the index only
// on streams referenced by a sidx. Unused space is turned into a free box
// placed before the sidx boxes, so that the last sidx ends exactly where the
// first moof starts: FFmpeg-based players treat the index as complete only
// when the byte ranges it references start right after it and extend to the
// end of the file.
func (si *seekIndex) write(
	f io.WriteSeeker,
	segmentDuration time.Duration,
	tracks []*formatFMP4Track,
) error {
	reserved := si.reserved
	entries := si.entries

	if reserved <= 0 || len(entries) == 0 {
		return nil
	}

	// the track players seek on.
	refTrack := 0
	for i, track := range tracks {
		if track.initTrack.Codec.IsVideo() {
			refTrack = i
			break
		}
	}

	// group parts so that each reference starts with a sync sample of the
	// reference track: on a seek, FFmpeg-based players jump to the start of
	// the reference containing the target and can only start decoding from a
	// sync sample; if the reference doesn't start with one, they fall back to
	// scanning the file from the beginning.
	merged := make([]seekIndexEntry, 0, len(entries))
	for _, e := range entries {
		if len(merged) == 0 || (e.tracks[refTrack].present && e.tracks[refTrack].sap) {
			merged = append(merged, e)
		} else {
			mergeSeekIndexEntries(&merged[len(merged)-1], e)
		}
	}
	entries = merged

	// coalesce entries in the unlikely case that the segment
	// contains more sync samples than planned.
	if len(entries) > reserved {
		group := (len(entries) + reserved - 1) / reserved
		merged = make([]seekIndexEntry, 0, reserved)
		for i := 0; i < len(entries); i += group {
			e := entries[i]
			for j := i + 1; j < min(i+group, len(entries)); j++ {
				mergeSeekIndexEntries(&e, entries[j])
			}
			merged = append(merged, e)
		}
		entries = merged
	}

	// referenced_size is 31 bits; give up rather than write a broken index.
	for _, e := range entries {
		if e.size > math.MaxInt32 {
			return nil
		}
	}

	boxSize := seekIndexBoxSize(len(entries))
	leftover := len(tracks) * (seekIndexBoxSize(reserved) - boxSize)

	var buf []byte

	// turn unused space into a free box, placed before the sidx boxes.
	if leftover > 0 {
		buf = make([]byte, leftover, leftover+len(tracks)*boxSize)
		binary.BigEndian.PutUint32(buf, uint32(leftover))
		copy(buf[4:8], "free")
	}

	for trackIndex, track := range tracks {
		timeScale := track.initTrack.TimeScale
		total := uint64(multiplyAndDivide(int64(segmentDuration), int64(timeScale), int64(time.Second)))

		// start time of each reference on this track's timeline, taken from
		// the DTS of its first sample so that it matches the tfdt written in
		// the corresponding moof; references without samples of this track
		// take the start of the following one. Clamped to be non-decreasing.
		starts := make([]uint64, len(entries)+1)
		starts[len(entries)] = total
		for i, e := range slices.Backward(entries) {
			if e.tracks[trackIndex].present {
				starts[i] = e.tracks[trackIndex].baseTime
			} else {
				starts[i] = starts[i+1]
			}
		}
		for i := 1; i <= len(entries); i++ {
			if starts[i] < starts[i-1] {
				starts[i] = starts[i-1]
			}
		}

		sidx := amp4.Sidx{
			FullBox:                    amp4.FullBox{Version: 1},
			ReferenceID:                uint32(track.initTrack.ID),
			Timescale:                  timeScale,
			EarliestPresentationTimeV1: starts[0],
			FirstOffsetV1:              uint64((len(tracks) - 1 - trackIndex) * boxSize),
			ReferenceCount:             uint16(len(entries)),
		}

		for i, e := range entries {
			ref := amp4.SidxReference{
				ReferencedSize:     uint32(e.size),
				SubsegmentDuration: uint32(starts[i+1] - starts[i]),
			}
			if e.tracks[trackIndex].present && e.tracks[trackIndex].sap {
				ref.StartsWithSAP = true
				ref.SAPType = 1
			}

			sidx.References = append(sidx.References, ref)
		}

		var payload bytes.Buffer
		_, err := amp4.Marshal(&payload, &sidx, amp4.Context{})
		if err != nil {
			return err
		}

		sidxHeader := make([]byte, 8)
		binary.BigEndian.PutUint32(sidxHeader, uint32(8+payload.Len()))
		copy(sidxHeader[4:8], "sidx")
		buf = append(buf, sidxHeader...)
		buf = append(buf, payload.Bytes()...)
	}

	_, err := f.Seek(si.pos, io.SeekStart)
	if err != nil {
		return err
	}

	_, err = f.Write(buf)
	return err
}
