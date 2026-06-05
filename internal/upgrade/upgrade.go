// Package upgrade contains functions to upgrade the executable.
package upgrade

import (
	"bytes"
	"fmt"
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

// Upgrade downloads the latest executable and replaces the current one with it.
func Upgrade(version, arch string) error {
	if !currentRegexp.MatchString(version) {
		return fmt.Errorf("current version (%v) is not official and cannot be upgraded", version)
	}

	fmt.Println("getting latest version...")

	latest, err := latestRemoteVersion()
	if err != nil {
		return err
	}

	current, _ := semver.NewVersion(version)

	if current.GreaterThanEqual(latest) {
		fmt.Printf("current version (%v) is up to date\n", "v"+current.String())
		return nil
	}

	fmt.Printf("downloading version %v...\n", "v"+latest.String())

	ur := fmt.Sprintf(downloadURL, "v"+latest.String(), "v"+latest.String(), runtime.GOOS, arch, extension)

	res, err := http.Get(ur)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	exe, err := extractExecutable(res.Body)
	if err != nil {
		return err
	}

	err = selfupdate.Apply(bytes.NewReader(exe), selfupdate.Options{})
	if err != nil {
		return err
	}

	fmt.Printf("MediaMTX upgraded successfully from %v to %v.\n", "v"+current.String(), "v"+latest.String())
	return nil
}
