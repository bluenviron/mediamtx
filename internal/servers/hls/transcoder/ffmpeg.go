package transcoder

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	defaultVideoCodec      = "libx264"
	defaultAudioCodec      = "aac"
	defaultPreset          = "veryfast"
	defaultRTMPAddress     = ":1935"
	defaultFPS             = "30"
	defaultPixelFormat     = "yuv420p"
	defaultAudioBitrate    = "128k"
	defaultAudioSampleRate = "48000"
	defaultAudioChannels   = "2"
	audioResampleFilter    = "aresample=async=1:first_pts=0"
	x264ClosedGOPParams    = "scenecut=0:open_gop=0:rc-lookahead=0"

	// Packets queued between the demuxer thread and FFmpeg, and per output
	// between the encoder and the RTSP muxer.
	inputThreadQueueSize  = "512"
	outputMuxingQueueSize = "1024"
	maxrateFactor         = 1.1
	bufsizeFactor         = 2.0
)

type FFmpegTranscoder struct {
	Conf            *conf.Path
	StreamID        string
	Parent          logger.Writer
	rtmpAddress     string
	rtspAddress     string
	segmentDuration time.Duration
	SourceInfo      *SourceInfo

	ctx       context.Context
	ctxCancel context.CancelFunc

	cmd      *exec.Cmd
	cmdMutex sync.Mutex
	done     chan struct{}
}

func NewFFmpegTranscoder(cfg *conf.Path, streamID string, parent logger.Writer, rtmpAddress string, rtspAddress string, segmentDuration time.Duration) *FFmpegTranscoder {
	ctx, cancel := context.WithCancel(context.Background())
	if strings.TrimSpace(rtmpAddress) == "" {
		rtmpAddress = defaultRTMPAddress
	}
	if segmentDuration <= 0 {
		segmentDuration = 2 * time.Second
	}
	return &FFmpegTranscoder{
		Conf:            cfg,
		StreamID:        streamID,
		Parent:          parent,
		rtmpAddress:     rtmpAddress,
		rtspAddress:     rtspAddress,
		segmentDuration: segmentDuration,
		ctx:             ctx,
		ctxCancel:       cancel,
	}
}

func (t *FFmpegTranscoder) rtspPort() string {
	if t.rtspAddress != "" {
		_, port, err := net.SplitHostPort(t.rtspAddress)
		if err == nil && port != "" {
			return port
		}
	}
	return "8554"
}

func (t *FFmpegTranscoder) Log(level logger.Level, format string, args ...interface{}) {
	t.Parent.Log(level, "[Transcoder %s] "+format, append([]interface{}{t.StreamID}, args...)...)
}

// Backoff between FFmpeg restarts: it doubles from restartMinBackoff up to
// restartMaxBackoff, and starts over once a process has run for
// restartStableAfter. Variables so that tests can shorten them.
var (
	restartMinBackoff  = 1 * time.Second
	restartMaxBackoff  = 30 * time.Second
	restartStableAfter = 60 * time.Second
)

// Start runs FFmpeg for the stream and keeps it running until Stop.
//
// FFmpeg exits on its own while the stream is still published when it falls
// behind: the server drops frames to it ("reader is too slow") and then closes
// its input ("too many reordered frames"), and FFmpeg stops on the broken pipe.
// Its renditions went with it for the rest of the stream, and players kept
// asking for them. It is now started again after a backoff, so the renditions
// come back.
func (t *FFmpegTranscoder) Start() error {
	t.Log(logger.Info, "Starting FFmpeg transcoder for stream %s", t.StreamID)

	cmd, stderr, err := t.startProcess()
	if err != nil {
		return err
	}

	done := make(chan struct{})
	t.cmdMutex.Lock()
	t.done = done
	t.cmdMutex.Unlock()

	go t.supervise(done, cmd, stderr)
	return nil
}

func (t *FFmpegTranscoder) startProcess() (*exec.Cmd, io.ReadCloser, error) {
	cmd := exec.CommandContext(t.ctx, "ffmpeg", t.BuildArgs()...)
	cmd.SysProcAttr = processGroupSysProcAttr()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	t.cmdMutex.Lock()
	t.cmd = cmd
	t.cmdMutex.Unlock()

	return cmd, stderr, nil
}

// supervise waits for the running FFmpeg and restarts it until Stop.
func (t *FFmpegTranscoder) supervise(done chan struct{}, cmd *exec.Cmd, stderr io.ReadCloser) {
	defer close(done)

	backoff := restartMinBackoff

	for {
		started := time.Now()

		if cmd != nil {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				t.Log(logger.Warn, "FFmpeg: %s", scanner.Text())
			}

			// Wait only after the scanner has read all of stderr (EOF).
			err := cmd.Wait()

			if t.ctx.Err() != nil {
				t.Log(logger.Info, "FFmpeg transcoder stopped")
				return
			}

			t.Log(logger.Error, "FFmpeg transcoder exited with the stream still live (%v)", err)

			if time.Since(started) >= restartStableAfter {
				backoff = restartMinBackoff
			}
		}

		t.Log(logger.Info, "restarting FFmpeg transcoder in %v", backoff)

		select {
		case <-t.ctx.Done():
			t.Log(logger.Info, "FFmpeg transcoder stopped")
			return
		case <-time.After(backoff):
		}

		backoff = min(backoff*2, restartMaxBackoff)

		var err error
		cmd, stderr, err = t.startProcess()
		if err != nil {
			t.Log(logger.Error, "unable to restart FFmpeg transcoder: %v", err)
			cmd = nil
		}
	}
}

// BuildArgs builds FFmpeg arguments without starting the process.
func (t *FFmpegTranscoder) BuildArgs() []string {
	sourceURL := t.rtmpURL(t.StreamID)

	renditions := t.Conf.HLSTranscodingRenditions
	fps := defaultFPS
	if t.SourceInfo != nil && t.SourceInfo.FPS > 0 {
		fps = fmt.Sprintf("%.2f", t.SourceInfo.FPS)
	}
	segmentDurationSeconds := t.segmentDuration.Seconds()
	gopSize := gopSizeForFPS(fps, segmentDurationSeconds)
	filterGraph := t.videoFilterGraph(renditions, fps)

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "warning",
		"-err_detect", "ignore_err",
		"-fflags", "nobuffer+fastseek+genpts+discardcorrupt",
		"-flags", "low_delay",
		"-analyzeduration", "5000000",
		"-probesize", "5000000",
		// Read the input on its own thread so a short stall in the filter or
		// encoder does not make mediamtx drop frames for this reader.
		"-thread_queue_size", inputThreadQueueSize,
		"-i", sourceURL,
		"-filter_complex", filterGraph,
	}

	videoCodec := defaultString(t.Conf.HLSTranscodingVideoCodec, defaultVideoCodec)
	preset := defaultString(t.Conf.HLSTranscodingPreset, defaultPreset)

	audioCodec := defaultString(t.Conf.HLSTranscodingAudioCodec, defaultAudioCodec)
	audioMap := "0:a:0?"
	if t.SourceInfo != nil && !t.SourceInfo.HasAudio() {
		args = append(args,
			"-f", "lavfi",
			"-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		)
		audioMap = "1:a:0"
	}

	// Transcoded renditions from config (each with its own video and audio muxed)
	for _, r := range renditions {
		outURL := fmt.Sprintf("rtsp://127.0.0.1:%s/%s/%s", t.rtspPort(), t.StreamID, r.Name)
		args = append(args,
			"-map", fmt.Sprintf("[out%s]", r.Name),
			"-c:v", videoCodec,
			"-pix_fmt", defaultPixelFormat,
			"-b:v", r.VideoBitrate,
		)
		// Cap bursts on complex scenes so one rendition cannot spike the CPU
		// and the RTSP output.
		if maxrate, bufsize, ok := rateLimits(r.VideoBitrate); ok {
			args = append(args, "-maxrate", maxrate, "-bufsize", bufsize)
		}
		args = append(args,
			"-preset", preset,
			"-tune", "zerolatency",
			"-g", gopSize,
			"-keyint_min", gopSize,
			"-sc_threshold", "0",
			"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%s)", formatDurationSeconds(segmentDurationSeconds)),
			"-x264-params", x264ClosedGOPParams,
			"-map", audioMap,
			"-c:a", audioCodec,
			"-b:a", defaultAudioBitrate,
			"-ar", defaultAudioSampleRate,
			"-ac", defaultAudioChannels,
			"-max_muxing_queue_size", outputMuxingQueueSize,
			"-f", "rtsp",
			"-rtsp_transport", "tcp",
			outURL,
		)
	}

	return args
}

// rateLimits derives -maxrate and -bufsize from a bitrate such as "3000k" or
// "2M". ok is false when the bitrate is not in that form, so no limit is set.
func rateLimits(bitrate string) (maxrate, bufsize string, ok bool) {
	b := strings.TrimSpace(bitrate)
	if len(b) < 2 {
		return "", "", false
	}

	unit := b[len(b)-1]
	if unit != 'k' && unit != 'K' && unit != 'm' && unit != 'M' {
		return "", "", false
	}

	v, err := strconv.ParseFloat(b[:len(b)-1], 64)
	if err != nil || v <= 0 {
		return "", "", false
	}

	// Kilobits keep the numbers integral for both units.
	if unit == 'm' || unit == 'M' {
		v *= 1000
	}

	return fmt.Sprintf("%dk", int(v*maxrateFactor+0.5)),
		fmt.Sprintf("%dk", int(v*bufsizeFactor+0.5)),
		true
}

func gopSizeForFPS(fps string, segmentDurationSeconds float64) string {
	v, err := strconv.ParseFloat(strings.TrimSpace(fps), 64)
	if err != nil || v <= 0 {
		v, _ = strconv.ParseFloat(defaultFPS, 64)
	}
	if segmentDurationSeconds <= 0 {
		segmentDurationSeconds = 2
	}
	return strconv.Itoa(int(v*segmentDurationSeconds + 0.5))
}

func formatDurationSeconds(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (t *FFmpegTranscoder) rtmpURL(parts ...string) string {
	address := strings.TrimSpace(t.rtmpAddress)
	if address == "" {
		address = defaultRTMPAddress
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		_, port, _ = net.SplitHostPort(defaultRTMPAddress)
	}
	return "rtmp://127.0.0.1:" + port + "/" + strings.Join(parts, "/")
}

func (t *FFmpegTranscoder) videoFilterGraph(renditions []conf.HLSTranscodingRendition, fps string) string {
	var splitOuts []string
	for _, r := range renditions {
		splitOuts = append(splitOuts, fmt.Sprintf("[v%sin]", r.Name))
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("[0:v]fps=%s,setpts=PTS-STARTPTS,split=", fps))
	b.WriteString(strconv.Itoa(len(renditions)))
	b.WriteString(strings.Join(splitOuts, ""))
	b.WriteByte(';')

	for _, r := range renditions {
		b.WriteString(fmt.Sprintf(
			"[v%sin]scale=%d:%d:force_original_aspect_ratio=decrease:flags=fast_bilinear,pad=%d:%d:(ow-iw)/2:(oh-ih)/2[out%s];",
			r.Name, r.Width, r.Height, r.Width, r.Height, r.Name,
		))
	}

	return strings.TrimSuffix(b.String(), ";")
}

func defaultString(v string, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func (t *FFmpegTranscoder) Stop() {
	t.ctxCancel()

	t.cmdMutex.Lock()
	cmd := t.cmd
	done := t.done
	t.cmdMutex.Unlock()

	if cmd != nil && cmd.Process != nil && done != nil {
		killProcessGroup(cmd.Process.Pid)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			forceKillProcessGroup(cmd.Process.Pid)
			<-done
		}
	}
}

func processGroupSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGINT)
}

func forceKillProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// FilterRenditions removes renditions whose resolution exceeds the source.
// This prevents wasteful upscaling when source is lower than configured maximum.
func FilterRenditions(renditions []conf.HLSTranscodingRendition, source *SourceInfo) []conf.HLSTranscodingRendition {
	if source == nil || source.Height == 0 {
		return append([]conf.HLSTranscodingRendition(nil), renditions...)
	}

	var filtered []conf.HLSTranscodingRendition
	for _, r := range renditions {
		if r.Height <= source.Height {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		if len(renditions) == 0 {
			return nil
		}
		smallest := renditions[0]
		for _, r := range renditions[1:] {
			if r.Height < smallest.Height {
				smallest = r
			}
		}
		return []conf.HLSTranscodingRendition{smallest}
	}
	return filtered
}
