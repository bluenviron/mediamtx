package playback

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/record"
)

// Segment is a recording segment.
type Segment struct {
	fpath    string
	Start    time.Time
	duration time.Duration
}

func findSegmentsInTimespan(
	pathConf *conf.Path,
	pathName string,
	start time.Time,
	duration time.Duration,
) ([]*Segment, error) {
	if !pathConf.Playback {
		return nil, fmt.Errorf("playback is disabled on path '%s'", pathName)
	}

	recordPath := record.PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := record.CommonPath(recordPath)
	end := start.Add(duration)
	var segments []*Segment

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
			ok := pa.Decode(recordPath, fpath)

			// gather all segments that starts before the end of the playback
			if ok && !end.Before(pa.Start) {
				segments = append(segments, &Segment{
					fpath: fpath,
					Start: pa.Start,
				})
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if segments == nil {
		return nil, errNoSegmentsFound
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Start.Before(segments[j].Start)
	})

	// find the segment that may contain the start of the playback and remove all previous ones
	found := false
	for i := 0; i < len(segments)-1; i++ {
		if !start.Before(segments[i].Start) && start.Before(segments[i+1].Start) {
			segments = segments[i:]
			found = true
			break
		}
	}

	// otherwise, keep the last segment only and check if it may contain the start of the playback
	if !found {
		segments = segments[len(segments)-1:]
		if segments[len(segments)-1].Start.After(start) {
			return nil, errNoSegmentsFound
		}
	}

	return segments, nil
}

// FindSegments returns all segments of a path.
func FindSegments(
	pathConf *conf.Path,
	pathName string,
) ([]*Segment, error) {
	if !pathConf.Playback {
		return nil, fmt.Errorf("playback is disabled on path '%s'", pathName)
	}

	recordPath := record.PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := record.CommonPath(recordPath)
	var segments []*Segment

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
			ok := pa.Decode(recordPath, fpath)
			if ok {
				segments = append(segments, &Segment{
					fpath: fpath,
					Start: pa.Start,
				})
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if segments == nil {
		return nil, errNoSegmentsFound
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Start.Before(segments[j].Start)
	})

	return segments, nil
}

func canBeConcatenated(seg1, seg2 *Segment) bool {
	end1 := seg1.Start.Add(seg1.duration)
	return !seg2.Start.Before(end1.Add(-concatenationTolerance)) && !seg2.Start.After(end1.Add(concatenationTolerance))
}

func mergeConcatenatedSegments(in []*Segment) []*Segment {
	var out []*Segment

	for _, seg := range in {
		if len(out) != 0 && canBeConcatenated(out[len(out)-1], seg) {
			start := out[len(out)-1].Start
			end := seg.Start.Add(seg.duration)
			out[len(out)-1].duration = end.Sub(start)
		} else {
			out = append(out, seg)
		}
	}

	return out
}
