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
	log.Println("downloading hls.js...")

	buf, err := os.ReadFile("./hlsjsdownloader/VERSION")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(buf))

	res, err := http.Get("https://cdn.jsdelivr.net/npm/hls.js@" + version + "/dist/hls.min.js")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	buf, err = io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	err = os.WriteFile("hls.min.js", buf, 0o644)
	if err != nil {
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
