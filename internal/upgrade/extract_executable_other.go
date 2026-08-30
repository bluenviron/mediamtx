//go:build !windows

package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
)

const extension = "tar.gz"

func extractExecutable(r io.Reader) ([]byte, error) {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close() //nolint:errcheck

	tarReader := tar.NewReader(gzReader)

	for {
		var header *tar.Header
		header, err = tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("executable not found")
			}
			return nil, err
		}

		if header.Name == executable {
			var buf []byte
			buf, err = io.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}
			return buf, nil
		}
	}
}
