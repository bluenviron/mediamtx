//go:build enable_upgrade

package core

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/minio/selfupdate"
)

const (
	gitRepo     = "https://github.com/bluenviron/mediamtx"
	downloadURL = "https://github.com/bluenviron/mediamtx/releases/download/%s/mediamtx_%s_%s_%s.%s"
	executable  = "mediamtx"
)

var (
	tagsRegexp    = regexp.MustCompile(`^refs/tags/(v1\.[0-9]+\.[0-9]+)$`)
	currentRegexp = regexp.MustCompile(`^(v1\.[0-9]+\.[0-9]+)$`)
)

func latestRemoteVersion() (*semver.Version, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{gitRepo},
	})

	refs, err := rem.List(&git.ListOptions{})
	if err != nil {
		return nil, err
	}

	var versions []*semver.Version

	for _, ref := range refs {
		matches := tagsRegexp.FindStringSubmatch(ref.Name().String())
		if matches != nil {
			v, _ := semver.NewVersion(matches[1])
			versions = append(versions, v)
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found")
	}

	sort.Sort(sort.Reverse(semver.Collection(versions)))

	return versions[0], nil
}

func extractExecutable(r io.Reader) ([]byte, error) {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("executable not found")
		}
		if err != nil {
			return nil, err
		}

		if header.Name == executable {
			buf, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}
			return buf, nil
		}
	}
}

func extractExecutableWin(r io.Reader) ([]byte, error) {
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
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			buf, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			return buf, nil
		}
	}

	return nil, fmt.Errorf("executable not found")
}

func upgrade() error {
	if !currentRegexp.MatchString(string(version)) {
		return fmt.Errorf("current version (%v) is not official and cannot be upgraded", string(version))
	}

	fmt.Println("getting latest version...")

	latest, err := latestRemoteVersion()
	if err != nil {
		return err
	}

	current, _ := semver.NewVersion(string(version))

	if current.GreaterThanEqual(latest) {
		fmt.Printf("current version (%v) is up to date\n", "v"+current.String())
		return nil
	}

	fmt.Printf("downloading version %v...\n", "v"+latest.String())

	var extension string
	if runtime.GOOS == "windows" {
		extension = "zip"
	} else {
		extension = "tar.gz"
	}

	ur := fmt.Sprintf(downloadURL, "v"+latest.String(), "v"+latest.String(), runtime.GOOS, getArch(), extension)

	res, err := http.Get(ur)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	var exe []byte
	if runtime.GOOS == "windows" {
		exe, err = extractExecutableWin(res.Body)
	} else {
		exe, err = extractExecutable(res.Body)
	}

	err = selfupdate.Apply(bytes.NewReader(exe), selfupdate.Options{})
	if err != nil {
		return err
	}

	fmt.Printf("MediaMTX upgraded successfully from %v to %v.\n", "v"+current.String(), "v"+latest.String())
	return nil
}
