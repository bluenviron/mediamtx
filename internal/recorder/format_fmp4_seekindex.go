package recorder

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"slices"
	"time"

	amp4 "github.com/abema/go-mp4"
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

// writeSeekIndex overwrites the placeholder written by
// writeSeekIndexPlaceholder with one sidx box per track, allowing players to
// seek without scanning the whole file. A sidx per track is needed since
// FFmpeg-based players seek through the index only on streams referenced by
// a sidx. Unused space is turned into a free box placed before the sidx
// boxes, so that the last sidx ends exactly where the first moof starts:
// FFmpeg-based players treat the index as complete only when the byte ranges
// it references start right after it and extend to the end of the file.
func writeSeekIndex(
	f io.WriteSeeker,
	pos int64,
	reserved int,
	entries []seekIndexEntry,
	segmentDuration time.Duration,
	tracks []*formatFMP4Track,
) error {
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

	_, err := f.Seek(pos, io.SeekStart)
	if err != nil {
		return err
	}

	_, err = f.Write(buf)
	return err
}
