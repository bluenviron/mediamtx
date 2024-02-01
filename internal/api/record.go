package api

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/playback"
	"github.com/bluenviron/mediamtx/internal/record"
)

var errFound = errors.New("found")

func fixedPathHasRecordings(pathConf *conf.Path) bool {
	recordPath := record.PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathConf.Name),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := record.CommonPath(recordPath)

	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
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

func regexpPathGetRecordings(pathConf *conf.Path) []string {
	recordPath := record.PathAddExtension(
		pathConf.RecordPath,
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := record.CommonPath(recordPath)

	var ret []string

	filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
			ok := pa.Decode(recordPath, fpath)
			if ok && pathConf.Regexp.FindStringSubmatch(pa.Path) != nil {
				ret = append(ret, pa.Path)
			}
		}

		return nil
	})

	return ret
}

func removeDuplicatesAndSort(in []string) []string {
	ma := make(map[string]struct{}, len(in))
	for _, i := range in {
		ma[i] = struct{}{}
	}

	out := []string{}

	for k := range ma {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func getAllPathsWithRecordings(paths map[string]*conf.Path) []string {
	pathNames := []string{}

	for _, pathConf := range paths {
		if pathConf.Playback {
			if pathConf.Regexp == nil {
				if fixedPathHasRecordings(pathConf) {
					pathNames = append(pathNames, pathConf.Name)
				}
			} else {
				pathNames = append(pathNames, regexpPathGetRecordings(pathConf)...)
			}
		}
	}

	return removeDuplicatesAndSort(pathNames)
}

func recordingEntry(
	pathConf *conf.Path,
	pathName string,
) *defs.APIRecording {
	ret := &defs.APIRecording{
		Name: pathName,
	}

	segments, _ := playback.FindSegments(pathConf, pathName)

	ret.Segments = make([]*defs.APIRecordingSegment, len(segments))

	for i, seg := range segments {
		ret.Segments[i] = &defs.APIRecordingSegment{
			Start: seg.Start,
		}
	}

	return ret
}
