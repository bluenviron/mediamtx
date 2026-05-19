package upgrade

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// CheckVersion checks whether a new version is available.
// Returns true if a newer version is available.
func CheckVersion(version, _ string) (bool, error) {
	if !currentRegexp.MatchString(version) {
		return false, fmt.Errorf("current version (%v) is not official and cannot be checked", version)
	}

	fmt.Println("getting latest version...")

	latest, err := latestRemoteVersion()
	if err != nil {
		return false, err
	}

	current, _ := semver.NewVersion(version)

	if current.GreaterThanEqual(latest) {
		fmt.Printf("current version (%v) is up to date\n", "v"+current.String())
		return false, nil
	}

	fmt.Printf("a new version is available: %v (current: %v)\n", "v"+latest.String(), "v"+current.String())
	return true, nil
}
