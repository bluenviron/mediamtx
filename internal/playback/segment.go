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

type segment struct {
	fpath    string
	start    time.Time
	duration time.Duration
}

func findSegments(
	pathConf *conf.Path,
	pathName string,
	start time.Time,
	duration time.Duration,
) ([]*segment, error) {
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
	var segments []*segment

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
			ok := pa.Decode(recordPath, fpath)

			// gather all segments that starts before the end of the playback
			if ok && !end.Before(time.Time(pa)) {
				segments = append(segments, &segment{
					fpath: fpath,
					start: time.Time(pa),
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
		return segments[i].start.Before(segments[j].start)
	})

	// find the segment that may contain the start of the playback and remove all previous ones
	found := false
	for i := 0; i < len(segments)-1; i++ {
		if !start.Before(segments[i].start) && start.Before(segments[i+1].start) {
			segments = segments[i:]
			found = true
			break
		}
	}

	// otherwise, keep the last segment only and check if it may contain the start of the playback
	if !found {
		segments = segments[len(segments)-1:]
		if segments[len(segments)-1].start.After(start) {
			return nil, errNoSegmentsFound
		}
	}

	return segments, nil
}

func findAllSegments(
	pathConf *conf.Path,
	pathName string,
) ([]*segment, error) {
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
	var segments []*segment

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
			ok := pa.Decode(recordPath, fpath)
			if ok {
				segments = append(segments, &segment{
					fpath: fpath,
					start: time.Time(pa),
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
		return segments[i].start.Before(segments[j].start)
	})

	return segments, nil
}

func canBeConcatenated(seg1, seg2 *segment) bool {
	end1 := seg1.start.Add(seg1.duration)
	return !seg2.start.Before(end1.Add(-concatenationTolerance)) && !seg2.start.After(end1.Add(concatenationTolerance))
}

func mergeConcatenatedSegments(in []*segment) []*segment {
	var out []*segment

	for _, seg := range in {
		if len(out) != 0 && canBeConcatenated(out[len(out)-1], seg) {
			start := out[len(out)-1].start
			end := seg.start.Add(seg.duration)
			out[len(out)-1].duration = end.Sub(start)
		} else {
			out = append(out, seg)
		}
	}

	return out
}
