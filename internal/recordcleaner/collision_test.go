package recordcleaner

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCleanerCollisionSegments(t *testing.T) {
	for _, format := range []struct {
		name  string
		value conf.RecordFormat
	}{
		{"fmp4", conf.RecordFormatFMP4},
		{"mpegts", conf.RecordFormatMPEGTS},
	} {
		for _, layout := range []string{"fixed", "regexp", "regexp_path_last"} {
			t.Run(format.name+"/"+layout, func(t *testing.T) {
				now := time.Date(2026, 1, 2, 12, 0, 0, 123456000, time.Local)
				dir := t.TempDir()
				pathConf := &conf.Path{
					Name:              "camera",
					RecordPath:        filepath.Join(dir, "%path", "%Y-%m-%d_%H-%M-%S-%f"),
					RecordFormat:      format.value,
					RecordDeleteAfter: conf.Duration(time.Hour),
				}
				if layout != "fixed" {
					pathConf.Name = "~^camera$"
					pathConf.Regexp = regexp.MustCompile("^camera$")
				}
				if layout == "regexp_path_last" {
					pathConf.RecordPath = filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f_%path")
				}
				var expired []string
				retained := make(map[string][]byte)
				for i, start := range []time.Time{now.Add(-2 * time.Hour), now.Add(-time.Minute)} {
					base := (recordstore.Path{Start: start, Path: "camera"}).Encode(pathConf.RecordPath)
					for j, suffix := range []string{"", "~1", "~2"} {
						path := recordstore.PathAddExtension(base+suffix, format.value)
						require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
						content := []byte{byte(i), byte(j)}
						require.NoError(t, os.WriteFile(path, content, 0o600))
						if i == 0 {
							expired = append(expired, path)
						} else {
							retained[path] = content
						}
					}
				}
				cleaner := &Cleaner{
					PathConfs: map[string]*conf.Path{pathConf.Name: pathConf},
					Parent:    test.NilLogger,
				}
				require.Equal(t, []string{"camera"}, recordstore.FindAllPathsWithSegments(cleaner.PathConfs))
				require.NoError(t, cleaner.processPath(now, "camera"))
				for _, path := range expired {
					require.NoFileExists(t, path)
				}
				for path, content := range retained {
					actual, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, content, actual)
				}
			})
		}
	}
}
