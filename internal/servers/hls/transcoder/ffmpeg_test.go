package transcoder

import (
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type mockLogger struct {
	t *testing.T
}

func (l *mockLogger) Log(level logger.Level, format string, args ...interface{}) {
	l.t.Logf(format, args...)
}

func TestTranscoderInit(t *testing.T) {
	cfg := &conf.Path{
		HLSTranscoding: true,
		HLSTranscodingRenditions: []conf.HLSTranscodingRendition{
			{Name: "720", Width: 1280, Height: 720, VideoBitrate: "3000k"},
		},
	}

	l := &mockLogger{t: t}
	tr := NewTranscoder(cfg, "test_stream", l, ":1935", "127.0.0.1:8554", 2*time.Second)

	if tr.StreamID != "test_stream" {
		t.Errorf("expected streamID test_stream, got %s", tr.StreamID)
	}
}

func TestFFmpegBuildArgsSharedAudioNestedOutputs(t *testing.T) {
	cfg := &conf.Path{
		HLSTranscoding: true,
		HLSTranscodingRenditions: []conf.HLSTranscodingRendition{
			{Name: "1080", Width: 1920, Height: 1080, VideoBitrate: "6000k"},
			{Name: "720", Width: 1280, Height: 720, VideoBitrate: "3000k"},
			{Name: "480", Width: 854, Height: 480, VideoBitrate: "1200k"},
		},
		HLSTranscodingVideoCodec: "libx264",
		HLSTranscodingAudioCodec: "aac",
		HLSTranscodingPreset:     "veryfast",
	}

	tr := NewFFmpegTranscoder(cfg, "cam1", &mockLogger{t: t}, ":1935", "127.0.0.1:8554", 2*time.Second)
	args := strings.Join(tr.BuildArgs(), " ")

	for _, expected := range []string{
		"-thread_queue_size 512 -i rtmp://127.0.0.1:1935/cam1",
		"fps=30,setpts=PTS-STARTPTS,split=3[v1080in][v720in][v480in]",
		"scale=1920:1080:force_original_aspect_ratio=decrease:flags=fast_bilinear,pad=1920:1080:(ow-iw)/2:(oh-ih)/2[out1080]",
		"scale=1280:720:force_original_aspect_ratio=decrease:flags=fast_bilinear,pad=1280:720:(ow-iw)/2:(oh-ih)/2[out720]",
		"scale=854:480:force_original_aspect_ratio=decrease:flags=fast_bilinear,pad=854:480:(ow-iw)/2:(oh-ih)/2[out480]",
		"-map [out1080] -c:v libx264 -pix_fmt yuv420p -b:v 6000k -maxrate 6600k -bufsize 12000k -preset veryfast -tune zerolatency",
		"-g 60 -keyint_min 60 -sc_threshold 0 -force_key_frames expr:gte(t,n_forced*2) -x264-params scenecut=0:open_gop=0:rc-lookahead=0",
		"-max_muxing_queue_size 1024 -f rtsp -rtsp_transport tcp rtsp://127.0.0.1:8554/cam1/1080",
		"rtsp://127.0.0.1:8554/cam1/720",
		"rtsp://127.0.0.1:8554/cam1/480",
	} {
		if !strings.Contains(args, expected) {
			t.Fatalf("args missing %q:\n%s", expected, args)
		}
	}
}

func TestFFmpegBuildArgsGOPMatchesSourceFPS(t *testing.T) {
	cfg := &conf.Path{
		HLSTranscoding: true,
		HLSTranscodingRenditions: []conf.HLSTranscodingRendition{
			{Name: "720", Width: 1280, Height: 720, VideoBitrate: "3000k"},
		},
	}

	tr := NewFFmpegTranscoder(cfg, "cam1", &mockLogger{t: t}, ":1935", "127.0.0.1:8554", 2*time.Second)
	tr.SourceInfo = &SourceInfo{FPS: 25}
	args := strings.Join(tr.BuildArgs(), " ")

	for _, expected := range []string{
		"fps=25.00,setpts=PTS-STARTPTS",
		"-g 50 -keyint_min 50 -sc_threshold 0 -force_key_frames expr:gte(t,n_forced*2)",
	} {
		if !strings.Contains(args, expected) {
			t.Fatalf("args missing %q:\n%s", expected, args)
		}
	}
}

func TestFFmpegBuildArgsGOPMatchesHLSSegmentDuration(t *testing.T) {
	cfg := &conf.Path{
		HLSTranscoding: true,
		HLSTranscodingRenditions: []conf.HLSTranscodingRendition{
			{Name: "720", Width: 1280, Height: 720, VideoBitrate: "3000k"},
		},
	}

	tr := NewFFmpegTranscoder(cfg, "cam1", &mockLogger{t: t}, ":1935", "127.0.0.1:8554", 1500*time.Millisecond)
	tr.SourceInfo = &SourceInfo{FPS: 30}
	args := strings.Join(tr.BuildArgs(), " ")

	for _, expected := range []string{
		"-g 45 -keyint_min 45 -sc_threshold 0 -force_key_frames expr:gte(t,n_forced*1.5)",
	} {
		if !strings.Contains(args, expected) {
			t.Fatalf("args missing %q:\n%s", expected, args)
		}
	}
}

func TestFilterRenditionsBySourceHeight(t *testing.T) {
	renditions := []conf.HLSTranscodingRendition{
		{Name: "1080", Width: 1920, Height: 1080, VideoBitrate: "6000k"},
		{Name: "720", Width: 1280, Height: 720, VideoBitrate: "3000k"},
		{Name: "480", Width: 854, Height: 480, VideoBitrate: "1200k"},
	}

	for _, ca := range []struct {
		name   string
		info   *SourceInfo
		wanted []string
	}{
		{"1080 source", &SourceInfo{Height: 1080}, []string{"1080", "720", "480"}},
		{"720 source", &SourceInfo{Height: 720}, []string{"720", "480"}},
		{"unknown source", nil, []string{"1080", "720", "480"}},
		{"zero height", &SourceInfo{}, []string{"1080", "720", "480"}},
		{"tiny source", &SourceInfo{Height: 240}, []string{"480"}},
	} {
		t.Run(ca.name, func(t *testing.T) {
			got := FilterRenditions(renditions, ca.info)
			if len(got) != len(ca.wanted) {
				t.Fatalf("expected %d renditions, got %+v", len(ca.wanted), got)
			}
			for i, name := range ca.wanted {
				if got[i].Name != name {
					t.Fatalf("expected rendition %d to be %q, got %+v", i, name, got)
				}
			}
		})
	}
}

func TestRateLimits(t *testing.T) {
	for _, tt := range []struct {
		in, maxrate, bufsize string
		ok                   bool
	}{
		{"6000k", "6600k", "12000k", true},
		{"1200k", "1320k", "2400k", true},
		{"2M", "2200k", "4000k", true},
		{"1.5M", "1650k", "3000k", true},
		{"", "", "", false},
		{"3000", "", "", false},
		{"0k", "", "", false},
		{"abck", "", "", false},
	} {
		maxrate, bufsize, ok := rateLimits(tt.in)
		if ok != tt.ok || maxrate != tt.maxrate || bufsize != tt.bufsize {
			t.Errorf("rateLimits(%q) = %q %q %v, want %q %q %v",
				tt.in, maxrate, bufsize, ok, tt.maxrate, tt.bufsize, tt.ok)
		}
	}
}
