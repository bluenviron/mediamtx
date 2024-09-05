package recordstore

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// ErrNoSegmentsFound is returned when no recording segments have been found.
var ErrNoSegmentsFound = errors.New("no recording segments found")

var errFound = errors.New("found")

// Segment is a recording segment.
type Segment struct {
	Fpath string
	Start time.Time
}

func fixedPathHasSegments(pathConf *conf.Path) bool {
	recordPath := PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathConf.Name),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := CommonPath(recordPath)

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa Path
			ok := pa.Decode(recordPath, fpath)
			if ok {
				return errFound
			}
		}

		return nil
	})
	if err != nil && !errors.Is(err, errFound) {
		return false
	}

	return errors.Is(err, errFound)
}

func regexpPathFindPathsWithSegments(pathConf *conf.Path) map[string]struct{} {
	recordPath := PathAddExtension(
		pathConf.RecordPath,
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := CommonPath(recordPath)

	ret := make(map[string]struct{})

	filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa Path
			ok := pa.Decode(recordPath, fpath)
			if ok && pathConf.Regexp.FindStringSubmatch(pa.Path) != nil {
				ret[pa.Path] = struct{}{}
			}
		}

		return nil
	})

	return ret
}

// FindAllPathsWithSegments returns all paths that do have segments.
func FindAllPathsWithSegments(pathConfs map[string]*conf.Path) []string {
	pathNames := make(map[string]struct{})

	for _, pathConf := range pathConfs {
		if pathConf.Regexp == nil {
			if fixedPathHasSegments(pathConf) {
				pathNames[pathConf.Name] = struct{}{}
			}
		} else {
			for name := range regexpPathFindPathsWithSegments(pathConf) {
				pathNames[name] = struct{}{}
			}
		}
	}

	out := make([]string, len(pathNames))
	n := 0
	for k := range pathNames {
		out[n] = k
		n++
	}
	sort.Strings(out)

	return out
}

// FindSegments returns all segments of a path.
func FindSegments(
	pathConf *conf.Path,
	pathName string,
) ([]*Segment, error) {
	recordPath := PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := CommonPath(recordPath)
	var segments []*Segment

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa Path
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
		return nil, ErrNoSegmentsFound
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Start.Before(segments[j].Start)
	})

	return segments, nil
}

// FindSegmentsInTimespan returns all segments in a certain timestamp.
func FindSegmentsInTimespan(
	pathConf *conf.Path,
	pathName string,
	start time.Time,
	duration time.Duration,
) ([]*Segment, error) {
	recordPath := PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := CommonPath(recordPath)
	end := start.Add(duration)
	var segments []*Segment

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa Path
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
		return nil, ErrNoSegmentsFound
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
			return nil, ErrNoSegmentsFound
		}
	}

	return segments, nil
}
