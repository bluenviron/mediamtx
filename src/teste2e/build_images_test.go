//go:build enable_e2e_tests

package teste2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func buildImage(image string) error {
	ecmd := exec.Command("docker", "build", filepath.Join("images", image),
		"-t", "mediamtx-test-"+image)
	ecmd.Stdout = nil
	ecmd.Stderr = os.Stderr
	return ecmd.Run()
}

func TestBuildImages(t *testing.T) {
	files, err := os.ReadDir("images")
	require.NoError(t, err)

	for _, file := range files {
		err := buildImage(file.Name())
		require.NoError(t, err)
	}
}
