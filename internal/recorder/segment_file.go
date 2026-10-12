package recorder

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func createSegmentFile(path string) (*os.File, error) {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)

	for i := 0; ; i++ {
		candidate := path
		if i != 0 {
			candidate = stem + "~" + strconv.Itoa(i) + ext
		}

		fi, err := os.OpenFile(candidate, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if !errors.Is(err, fs.ErrExist) {
			return fi, err
		}
	}
}
