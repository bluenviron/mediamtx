package playback

import (
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
	Fpath string
	Start time.Time
}

func findSegmentsInTimespan(
	pathConf *conf.Path,
	pathName string,
	start time.Time,
	duration time.Duration,
) ([]*Segment, error) {
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
					Fpath: fpath,
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
					Fpath: fpath,
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
