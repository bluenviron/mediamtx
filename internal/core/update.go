package core

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// GitHub API reponse for latest release
type Release struct {
	URL       string `json:"url"`
	AssetsURL string `json:"assets_url"`
	UploadURL string `json:"upload_url"`
	HTMLURL   string `json:"html_url"`
	ID        int    `json:"id"`
	Author    struct {
		Login             string `json:"login"`
		ID                int    `json:"id"`
		NodeID            string `json:"node_id"`
		AvatarURL         string `json:"avatar_url"`
		GravatarID        string `json:"gravatar_id"`
		URL               string `json:"url"`
		HTMLURL           string `json:"html_url"`
		FollowersURL      string `json:"followers_url"`
		FollowingURL      string `json:"following_url"`
		GistsURL          string `json:"gists_url"`
		StarredURL        string `json:"starred_url"`
		SubscriptionsURL  string `json:"subscriptions_url"`
		OrganizationsURL  string `json:"organizations_url"`
		ReposURL          string `json:"repos_url"`
		EventsURL         string `json:"events_url"`
		ReceivedEventsURL string `json:"received_events_url"`
		Type              string `json:"type"`
		SiteAdmin         bool   `json:"site_admin"`
	} `json:"author"`
	NodeID          string    `json:"node_id"`
	TagName         string    `json:"tag_name"`
	TargetCommitish string    `json:"target_commitish"`
	Name            string    `json:"name"`
	Draft           bool      `json:"draft"`
	Prerelease      bool      `json:"prerelease"`
	CreatedAt       time.Time `json:"created_at"`
	PublishedAt     time.Time `json:"published_at"`
	Assets          []struct {
		URL      string `json:"url"`
		ID       int    `json:"id"`
		NodeID   string `json:"node_id"`
		Name     string `json:"name"`
		Label    string `json:"label"`
		Uploader struct {
			Login             string `json:"login"`
			ID                int    `json:"id"`
			NodeID            string `json:"node_id"`
			AvatarURL         string `json:"avatar_url"`
			GravatarID        string `json:"gravatar_id"`
			URL               string `json:"url"`
			HTMLURL           string `json:"html_url"`
			FollowersURL      string `json:"followers_url"`
			FollowingURL      string `json:"following_url"`
			GistsURL          string `json:"gists_url"`
			StarredURL        string `json:"starred_url"`
			SubscriptionsURL  string `json:"subscriptions_url"`
			OrganizationsURL  string `json:"organizations_url"`
			ReposURL          string `json:"repos_url"`
			EventsURL         string `json:"events_url"`
			ReceivedEventsURL string `json:"received_events_url"`
			Type              string `json:"type"`
			SiteAdmin         bool   `json:"site_admin"`
		} `json:"uploader"`
		ContentType        string    `json:"content_type"`
		State              string    `json:"state"`
		Size               int       `json:"size"`
		DownloadCount      int       `json:"download_count"`
		CreatedAt          time.Time `json:"created_at"`
		UpdatedAt          time.Time `json:"updated_at"`
		BrowserDownloadURL string    `json:"browser_download_url"`
	} `json:"assets"`
	TarballURL string         `json:"tarball_url"`
	ZipballURL string         `json:"zipball_url"`
	Body       string         `json:"body"`
	Reactions  map[string]any `json:"reactions"`
}

func Update() {
	latestVersion := GetLatestRelease()

	fmt.Printf("Current version is %s, latest version is %s\n", version, latestVersion.TagName)

	if version == latestVersion.TagName {
		fmt.Println("Latest version already installed")
		return
	}

	// split major, minor and update numbers
	curVersionSplit := strings.Split(strings.TrimLeft(version, "v"), ".")
	newVersionSplit := strings.Split(strings.TrimLeft(latestVersion.TagName, "v"), ".")

	// Convert each string into an int
	var curVersionNumbers []int
	for _, x := range curVersionSplit {
		num, err := strconv.Atoi(x)
		if err != nil {
			fmt.Printf("ERR: update: %s\n", err.Error())
			os.Exit(1)
		}

		curVersionNumbers = append(curVersionNumbers, num)
	}

	var newVersionNumbers []int
	for _, x := range newVersionSplit {
		num, err := strconv.Atoi(x)
		if err != nil {
			fmt.Printf("ERR: update: %s\n", err.Error())
			os.Exit(1)
		}

		newVersionNumbers = append(newVersionNumbers, num)
	}

	// Compare if the version is newer
	if newVersionNumbers[0] > curVersionNumbers[0] ||
		newVersionNumbers[1] > curVersionNumbers[1] ||
		newVersionNumbers[2] > curVersionNumbers[2] {
		DownloadRelease(latestVersion)
	}
}

func GetLatestRelease() Release {
	resp, err := http.Get("https://api.github.com/repos/bluenviron/mediamtx/releases/latest")

	if err != nil {
		fmt.Printf("ERR: update: %s\n", err.Error())
		os.Exit(1)
	}

	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)

	if readErr != nil {
		fmt.Printf("ERR: update: %s\n", readErr.Error())
		os.Exit(1)
	}

	var LatestRelease Release
	jsonErr := json.Unmarshal(body, &LatestRelease)

	if jsonErr != nil {
		fmt.Printf("ERR: update: %s\n", jsonErr.Error())
		os.Exit(1)
	}

	return LatestRelease
}

func DownloadRelease(release Release) {
	fmt.Printf("Downloading version %s...\n", release.TagName)

	var DownloadUrl = ""

	for i := 0; i < len(release.Assets); i++ {
		asset := release.Assets[i]
		if strings.Contains(asset.Name, runtime.GOOS) && strings.Contains(asset.Name, runtime.GOARCH) {
			DownloadUrl = asset.BrowserDownloadURL
		}
	}

	if len(DownloadUrl) == 0 {
		fmt.Printf("No download version found for: %s %s", runtime.GOOS, runtime.GOARCH)
		os.Exit(1)
	}

	resp, err := http.Get(DownloadUrl)

	if err != nil {
		fmt.Printf("ERR: update: %s\n", err.Error())
		os.Exit(1)
	}

	defer resp.Body.Close()

	counter := &WriteCounter{Total: uint64(resp.ContentLength)}
	body, err := io.ReadAll(io.TeeReader(resp.Body, counter))

	fmt.Printf("\n")

	if err != nil {
		fmt.Printf("ERR: reading body: %s\n", err.Error())
		os.Exit(1)
	}

	var data []byte
	if strings.HasSuffix(DownloadUrl, ".zip") {
		data = GetApplicationFromZip(body)
	} else {
		data = GetApplicationFromTar(body)
	}

	updateErr := selfupdate.Apply(
		bytes.NewReader(data),
		selfupdate.Options{
			OldSavePath: GetExecutablePath() + ".old",
		})

	if updateErr != nil {
		fmt.Printf("ERR: patching: %s\n", updateErr.Error())
		os.Exit(1)
	}

	fmt.Println("Updated")
}

func GetApplicationFromZip(zipFile []byte) []byte {
	zp, err := zip.NewReader(bytes.NewReader(zipFile), int64(len(zipFile)))

	if err != nil {
		fmt.Printf("ERR: unzipping: %s\n", err.Error())
		os.Exit(1)
	}

	// Find the executable in the zip
	for i := 0; i < len(zp.File); i++ {
		file := zp.File[i]

		if file.Name != "LICENSE" && !strings.HasSuffix(file.Name, ".yml") {
			fileReader, err := file.Open()

			if err != nil {
				fmt.Printf("ERR: unzipping: %s\n", err.Error())
				os.Exit(1)
			}

			data, err := io.ReadAll(fileReader)
			if err != nil {
				fmt.Printf("ERR: unzipping: %s\n", err.Error())
				os.Exit(1)
			}
			return data
		}
	}

	// Should not be able to get here
	fmt.Printf("ERR: executable not found in zip: %s\n", err.Error())
	os.Exit(1)
	return make([]byte, 0)
}

func GetApplicationFromTar(tarFile []byte) []byte {
	uncompressedStream, err := gzip.NewReader(bytes.NewReader(tarFile))

	if err != nil {
		fmt.Printf("ERR: untarring: %s\n", err.Error())
		os.Exit(1)
	}

	tr := tar.NewReader(uncompressedStream)

	// Find the executable in the tar
	for {
		cur, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Printf("ERR: unzipping: %s\n", err.Error())
			os.Exit(1)
		}

		if cur.Typeflag != tar.TypeReg {
			continue
		}

		if cur.Name != "LICENSE" && !strings.HasSuffix(cur.Name, ".yml") {
			data, err := io.ReadAll(tr)
			if err != nil {
				fmt.Printf("ERR: unzipping: %s\n", err.Error())
				os.Exit(1)
			}

			return data
		}
	}

	// Should not be able to get here
	fmt.Printf("ERR: executable not found in tar: %s\n", err.Error())
	os.Exit(1)
	return make([]byte, 0)
}

func GetExecutablePath() string {
	ex, err := os.Executable()
	if err != nil {
		fmt.Printf("ERR: getting executable path: %s\n", err.Error())
		os.Exit(1)
	}
	return ex
}

// WriteCounter counts the number of bytes written to it. It implements to the io.Writer interface
// and we can pass this into io.TeeReader() which will report progress on each write cycle.
type WriteCounter struct {
	Downloaded uint64
	Total      uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += uint64(n)
	wc.PrintProgress()
	return n, nil
}

func (wc WriteCounter) PrintProgress() {
	// Clear the line by using a character return to go back to the start and remove
	// the remaining characters by filling it with spaces
	fmt.Printf("\r%s", strings.Repeat(" ", 35))

	// Return again and print current status of download
	// We use the humanize package to print the bytes in a meaningful way (e.g. 10 MB)
	percentage := float64(wc.Downloaded) / float64(wc.Total) * 100
	fmt.Printf("\r%v%% complete", math.Round(percentage))
}
