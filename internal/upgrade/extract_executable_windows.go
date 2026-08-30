package upgrade

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

const extension = "zip"

func extractExecutable(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	for _, file := range zipReader.File {
		if file.Name == executable+".exe" {
			var rc io.ReadCloser
			rc, err = file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			var buf []byte
			buf, err = io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			return buf, nil
		}
	}

	return nil, fmt.Errorf("executable not found")
}
