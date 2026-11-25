package core_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/core"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestAPIPathsGetRecording(t *testing.T) {
	config := "api: yes\nrtspTransports: [tcp]\nrtmp: no\nhls: no\nwebrtc: no\nsrt: no\nmoq: no\n" +
		"paths:\n" +
		"  mystream:\n" +
		"    alwaysAvailable: yes\n" +
		"    alwaysAvailableRecorded: no\n" +
		"    alwaysAvailableTracks:\n" +
		"      - codec: H264\n" +
		"    record: yes\n" +
		"    recordPath: " + strconv.Quote(filepath.Join(t.TempDir(), "%path/%Y-%m-%d_%H-%M-%S-%f%z")) + "\n"
	server, ok := core.New([]string{test.CreateTempFile(t, []byte(config))})
	require.True(t, ok)
	defer server.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	getRecording := func(t assert.TestingT) *bool {
		res, err := hc.Get("http://localhost:9997/v3/paths/get/mystream")
		if !assert.NoError(t, err) {
			return nil
		}
		defer res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		var out struct {
			Recording *bool `json:"recording"`
		}
		assert.NoError(t, json.NewDecoder(res.Body).Decode(&out))
		return out.Recording
	}

	require.Equal(t, new(false), getRecording(t))

	source := &gortsplib.Client{}
	err := source.StartRecording("rtsp://localhost:8554/mystream",
		&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
	require.NoError(t, err)
	defer source.Close()
	require.Equal(t, new(true), getRecording(t))

	source.Close()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, new(false), getRecording(c))
	}, 3*time.Second, 100*time.Millisecond)
}
