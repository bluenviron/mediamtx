package core

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

// recordedFiles returns the number and total size of the files recorded for a path.
// A missing directory means that nothing has been recorded yet.
func recordedFiles(t *testing.T, dir string) (int, int64) {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, 0
	}
	require.NoError(t, err)
	var size int64
	for _, f := range files {
		fi, err2 := os.Stat(filepath.Join(dir, f.Name()))
		require.NoError(t, err2)
		size += fi.Size()
	}
	return len(files), size
}

// publishH264 starts a RTSP publisher on the given path and returns the client
// and a function that writes one H264 packet, advancing the timestamp by tsStep.
func publishH264(t *testing.T, path string, tsStep uint32) (*gortsplib.Client, func() error) {
	medi := test.UniqueMediaH264()

	c := &gortsplib.Client{}
	err := c.StartRecording(
		"rtsp://localhost:8554/"+path,
		&description.Session{Medias: []*description.Media{medi}})
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	seq := uint16(1123)
	ts := uint32(45343)
	write := func() error {
		err2 := c.WritePacketRTP(medi, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: seq,
				Timestamp:      ts,
				SSRC:           563423,
			},
			Payload: []byte{5},
		})
		seq++
		ts += tsStep
		return err2
	}
	return c, write
}

func TestPathRecordAlwaysAvailableRecorded(t *testing.T) {
	dir := t.TempDir()
	recDir := filepath.Join(dir, "mystream")

	p, ok := newInstance(t,
		"record: yes\n"+
			"recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
			"paths:\n"+
			"  mystream:\n"+
			"    alwaysAvailable: yes\n"+
			"    alwaysAvailableRecorded: false\n"+
			"    alwaysAvailableTracks:\n"+
			"      - codec: H264\n")
	require.Equal(t, true, ok)
	defer p.Close()

	// while no source is connected, the offline segment must not be recorded
	time.Sleep(1 * time.Second)
	n, _ := recordedFiles(t, recDir)
	require.Equal(t, 0, n, "offline segment was recorded while no source was connected")

	// while a publisher is connected, recording must run
	source, write := publishH264(t, "mystream", 90000)
	for range 4 {
		require.NoError(t, write())
	}
	require.Eventually(t, func() bool {
		files, err := os.ReadDir(recDir)
		return err == nil && len(files) >= 1
	}, 10*time.Second, 100*time.Millisecond, "recording did not start after the publisher connected")

	// after the publisher disconnects, recording must stop:
	// no new files may appear and the finalized file must not keep growing
	source.Close()
	time.Sleep(1 * time.Second)
	n1, size1 := recordedFiles(t, recDir)
	require.Equal(t, 1, n1)

	time.Sleep(2 * time.Second)
	n2, size2 := recordedFiles(t, recDir)
	require.Equal(t, 1, n2, "recording continued after the publisher disconnected")
	require.Equal(t, size1, size2, "recording continued after the publisher disconnected")
}

func TestPathRecordAlwaysAvailableRecordedStaticSource(t *testing.T) {
	dir := t.TempDir()
	recDir := filepath.Join(dir, "mystream")

	p, ok := newInstance(t,
		"recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
			"paths:\n"+
			"  feeder:\n"+
			"  mystream:\n"+
			"    source: rtsp://localhost:8554/feeder\n"+
			"    record: yes\n"+
			"    alwaysAvailable: yes\n"+
			"    alwaysAvailableRecorded: false\n"+
			"    alwaysAvailableTracks:\n"+
			"      - codec: H264\n")
	require.Equal(t, true, ok)
	defer p.Close()

	// keep a publisher on the feeder path so that the static source can pull it
	feeder, write := publishH264(t, "feeder", 9000)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(100 * time.Millisecond):
				if write() != nil {
					return
				}
			}
		}
	}()

	// once the static source connects, recording must start
	require.Eventually(t, func() bool {
		files, err := os.ReadDir(recDir)
		return err == nil && len(files) >= 1
	}, 10*time.Second, 100*time.Millisecond, "recording did not start after the static source connected")

	// when the source disconnects, recording must stop while the offline segment plays
	feeder.Close()
	time.Sleep(1500 * time.Millisecond)
	n1, size1 := recordedFiles(t, recDir)

	time.Sleep(1500 * time.Millisecond)
	n2, size2 := recordedFiles(t, recDir)
	require.Equal(t, n1, n2, "recording continued after the static source disconnected")
	require.Equal(t, size1, size2, "recording continued after the static source disconnected")
}

func TestPathRecordAlwaysAvailableRecordedReloadWhileOffline(t *testing.T) {
	dir := t.TempDir()
	recDir := filepath.Join(dir, "mystream")

	p, ok := newInstance(t,
		"api: yes\n"+
			"recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
			"paths:\n"+
			"  feeder:\n"+
			"  mystream:\n"+
			"    source: rtsp://localhost:8554/feeder\n"+
			"    record: yes\n"+
			"    alwaysAvailable: yes\n"+
			"    alwaysAvailableRecorded: false\n"+
			"    alwaysAvailableTracks:\n"+
			"      - codec: H264\n")
	require.Equal(t, true, ok)
	defer p.Close()

	time.Sleep(500 * time.Millisecond)

	// a reload while the source is offline must not start recording the offline segment
	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/mystream", map[string]any{
		"recordDeleteAfter": "48h",
	}, nil)

	time.Sleep(2 * time.Second)

	n, _ := recordedFiles(t, recDir)
	require.Equal(t, 0, n, "recording started during a reload while the source was offline")
}
