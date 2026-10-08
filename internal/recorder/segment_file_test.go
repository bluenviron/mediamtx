package recorder

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateSegmentFile(t *testing.T) {
	for _, ext := range []string{".mp4", ".ts", ""} {
		t.Run(ext, func(t *testing.T) {
			stem := filepath.Join(t.TempDir(), "segment")
			path := stem + ext
			paths := make([]string, 0, 4)
			for i := range 4 {
				fi, err := createSegmentFile(path)
				require.NoError(t, err)
				paths = append(paths, fi.Name())
				if i == 0 {
					require.Equal(t, path, fi.Name())
				} else {
					require.Equal(t, stem+"~"+strconv.Itoa(i)+ext, fi.Name())
				}
				_, err = fi.Write([]byte{byte(i), 1, 2, 3})
				require.NoError(t, err)
				require.NoError(t, fi.Close())
			}

			for i, created := range paths {
				b, err := os.ReadFile(created)
				require.NoError(t, err)
				require.Equal(t, []byte{byte(i), 1, 2, 3}, b)
			}
		})
	}
}

func TestCreateSegmentFileConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "segment.mp4")
	const creators = 16
	type result struct {
		path    string
		payload byte
		err     error
	}
	results := make(chan result, creators)
	start := make(chan struct{})
	for i := range creators {
		go func() {
			<-start
			fi, err := createSegmentFile(path)
			if err != nil {
				results <- result{err: err}
				return
			}
			_, err = fi.Write([]byte{byte(i)})
			err = errors.Join(err, fi.Close())
			results <- result{path: fi.Name(), payload: byte(i), err: err}
		}()
	}
	close(start)

	paths := make(map[string]struct{})
	for range creators {
		res := <-results
		require.NoError(t, res.err)
		require.NotContains(t, paths, res.path)
		paths[res.path] = struct{}{}
		b, err := os.ReadFile(res.path)
		require.NoError(t, err)
		require.Equal(t, []byte{res.payload}, b)
	}
}

func TestCreateSegmentFileError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "segment.mp4")
	fi, err := createSegmentFile(path)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Nil(t, fi)
}
