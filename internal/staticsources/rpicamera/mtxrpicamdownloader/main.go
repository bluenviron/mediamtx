// Package main contains an utility to download hls.js
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func do() error {
	buf, err := os.ReadFile("./mtxrpicamdownloader/VERSION")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(buf))

	log.Printf("downloading mediamtx-rpicamera version %s...", version)

	for _, f := range []string{"mtxrpicam_32", "mtxrpicam_64"} {
		res, err := http.Get("https://github.com/bluenviron/mediamtx-rpicamera/releases/download/" + version + "/" + f)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("bad status code: %v", res.StatusCode)
		}

		buf, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		if err = os.WriteFile(f, buf, 0o644); err != nil {
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
