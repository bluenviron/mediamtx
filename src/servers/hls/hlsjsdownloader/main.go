// Package main contains an utility to download hls.js
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

func do() error {
	buf, err := os.ReadFile("./hlsjsdownloader/VERSION")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(buf))

	log.Printf("downloading hls.js %s...", version)

	res, err := http.Get("https://github.com/video-dev/hls.js/releases/download/" + version + "/release.zip")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	zipBuf, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	buf, err = os.ReadFile("./hlsjsdownloader/HASH")
	if err != nil {
		return err
	}
	str := strings.TrimSpace(string(buf))

	hash, err := hex.DecodeString(str)
	if err != nil {
		return err
	}

	if sum := sha256.Sum256(zipBuf); !bytes.Equal(sum[:], hash) {
		return fmt.Errorf("hash mismatch")
	}

	z, err := zip.NewReader(bytes.NewReader(zipBuf), int64(len(zipBuf)))
	if err != nil {
		return err
	}

	hls, err := fs.ReadFile(z, "dist/hls.min.js")
	if err != nil {
		return err
	}

	if err = os.WriteFile("hls.min.js", hls, 0o644); err != nil {
		return err
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
