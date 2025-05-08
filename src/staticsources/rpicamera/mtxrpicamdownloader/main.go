// Package main contains an utility to download hls.js
package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func dumpTar(src io.Reader) error {
	uncompressed, err := gzip.NewReader(src)
	if err != nil {
		return err
	}

	tr := tar.NewReader(uncompressed)

	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.Mkdir(header.Name, header.FileInfo().Mode())
			if err != nil {
				return err
			}

		case tar.TypeReg:
			f, err := os.OpenFile(header.Name, os.O_WRONLY|os.O_CREATE, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, tr)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func do() error {
	buf, err := os.ReadFile("./mtxrpicamdownloader/VERSION")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(buf))

	log.Printf("downloading mediamtx-rpicamera %s...", version)

	for _, f := range []string{"mtxrpicam_32.tar.gz", "mtxrpicam_64.tar.gz"} {
		err = os.RemoveAll(strings.TrimSuffix(f, ".tar.gz"))
		if err != nil {
			return err
		}

		res, err := http.Get("https://github.com/bluenviron/mediamtx-rpicamera/releases/download/" + version + "/" + f)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("bad status code: %v", res.StatusCode)
		}

		err = dumpTar(res.Body)
		if err != nil {
			return err
		}
	}

	log.Println("ok")
	return nil
}

func main() {
	err := do()
	if err != nil {
		log.Printf("ERR: %v", err)
		os.Exit(1)
	}
}
