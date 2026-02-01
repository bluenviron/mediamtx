// Package main provides a CLI that burns MediaMTX frame metadata into an MP4 as an overlay.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"

	"github.com/bluenviron/mediamtx/internal/framemetadata"
)

func usage() {
	fmt.Fprintln(os.Stderr, "Burn MediaMTX per-frame metadata into a recorded MP4 as a text overlay.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "This extracts metadata from:")
	fmt.Fprintln(os.Stderr, "  - H264/H265: SEI user_data_unregistered")
	fmt.Fprintln(os.Stderr, "  - AV1: METADATA OBU")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "It then generates an ASS subtitle file and calls ffmpeg to burn it.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Example:")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/mp4metaoverlay -in recordings/in.mp4 -out out.mp4")
	fmt.Fprintln(os.Stderr)
	flag.PrintDefaults()
}

func main() {
	var (
		inPath  = flag.String("in", "", "Input MP4 path")
		outPath = flag.String("out", "", "Output MP4 path")

		ffmpegPath = flag.String("ffmpeg", "ffmpeg", "ffmpeg binary path")
		vcodec     = flag.String("vcodec", "libx264", "Video encoder (ffmpeg -c:v)")
		crf        = flag.Int("crf", 18, "CRF (used by libx264/libx265)")
		preset     = flag.String("preset", "veryfast", "Encoder preset (used by libx264/libx265)")

		trackIndex = flag.Int("track", -1, "Video track index (0-based). If -1, picks first video track.")
	)
	flag.Usage = usage
	flag.Parse()

	if *inPath == "" || *outPath == "" {
		usage()
		os.Exit(2)
	}

	if err := run(*inPath, *outPath, *ffmpegPath, *vcodec, *crf, *preset, *trackIndex); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(inPath, outPath, ffmpegPath, vcodec string, crf int, preset string, trackIndex int) error {
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var pres pmp4.Presentation
	err = pres.Unmarshal(f)
	if err != nil {
		return err
	}

	tr, codecName, err := pickVideoTrack(&pres, trackIndex)
	if err != nil {
		return err
	}

	assPath, err := writeASSForTrack(tr, codecName)
	if err != nil {
		return err
	}
	defer os.Remove(assPath)

	// burn overlay
	// - re-encode video (required to burn)
	// - copy audio (if any)
	cmd := exec.Command(
		ffmpegPath,
		"-y",
		"-i", inPath,
		"-vf", "ass="+escapeFilterPath(assPath),
		"-c:v", vcodec,
		"-crf", fmt.Sprint(crf),
		"-preset", preset,
		"-c:a", "copy",
		"-movflags", "+faststart",
		outPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pickVideoTrack(p *pmp4.Presentation, idx int) (*pmp4.Track, string, error) {
	var videos []*pmp4.Track
	var names []string
	for _, tr := range p.Tracks {
		if tr.Codec == nil || !tr.Codec.IsVideo() {
			continue
		}
		switch tr.Codec.(type) {
		case *mp4.CodecH264:
			videos = append(videos, tr)
			names = append(names, "h264")
		case *mp4.CodecH265:
			videos = append(videos, tr)
			names = append(names, "h265")
		case *mp4.CodecAV1:
			videos = append(videos, tr)
			names = append(names, "av1")
		}
	}

	if len(videos) == 0 {
		return nil, "", errors.New("no supported video track found (supported: H264, H265, AV1)")
	}

	if idx < 0 {
		return videos[0], names[0], nil
	}
	if idx >= len(videos) {
		return nil, "", fmt.Errorf("track index out of range (have %d video tracks)", len(videos))
	}
	return videos[idx], names[idx], nil
}

func writeASSForTrack(tr *pmp4.Track, codecName string) (retPath string, retErr error) {
	tmp, err := os.CreateTemp("", "mediamtx-meta-*.ass")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	w := bufio.NewWriter(tmp)
	defer func() {
		if err2 := w.Flush(); err2 != nil && retErr == nil {
			retErr = err2
		}
	}()

	// Minimal ASS header.
	_, _ = io.WriteString(w, "[Script Info]\n")
	_, _ = io.WriteString(w, "ScriptType: v4.00+\n")
	_, _ = io.WriteString(w, "PlayResX: 1920\n")
	_, _ = io.WriteString(w, "PlayResY: 1080\n")
	_, _ = io.WriteString(w, "\n[V4+ Styles]\n")
	_, _ = io.WriteString(w,
		"Format: Name, Fontname, Fontsize, PrimaryColour, OutlineColour, BackColour, "+
			"Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, "+
			"BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	_, _ = io.WriteString(w,
		"Style: Default,DejaVu Sans,32,&H00FFFFFF,&H00000000,&H80000000,"+
			"0,0,0,0,100,100,0,0,1,2,1,7,40,40,40,1\n")
	_, _ = io.WriteString(w, "\n[Events]\n")
	_, _ = io.WriteString(w, "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	dts := int64(tr.TimeOffset)

	for i, sa := range tr.Samples {
		var payload []byte
		payload, err = sa.GetPayload()
		if err != nil {
			return "", err
		}

		pts := dts + int64(sa.PTSOffset)
		start := ticksToAssTime(pts, tr.TimeScale)

		endTicks := pts + int64(sa.Duration)
		// clamp end to next sample PTS if available (more accurate with B-frames)
		if i+1 < len(tr.Samples) {
			nextPTS := (dts + int64(sa.Duration)) + int64(tr.Samples[i+1].PTSOffset)
			if nextPTS > pts && nextPTS < endTicks {
				endTicks = nextPTS
			}
		}
		end := ticksToAssTime(endTicks, tr.TimeScale)

		text := overlayTextFromSample(codecName, payload)
		if text == "" {
			dts += int64(sa.Duration)
			continue
		}
		text = assEscape(text)

		_, err2 := fmt.Fprintf(w, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", start, end, text)
		if err2 != nil {
			return "", err2
		}

		dts += int64(sa.Duration)
	}

	return tmp.Name(), nil
}

func overlayTextFromSample(codecName string, sample []byte) string {
	switch codecName {
	case "h264":
		au := splitNALUs(sample)
		d, ok, _ := framemetadata.ExtractFromH264AU(au)
		if !ok {
			return ""
		}
		return framemetadata.FormatOverlayText(d)

	case "h265":
		au := splitNALUs(sample)
		d, ok, _ := framemetadata.ExtractFromH265AU(au)
		if !ok {
			return ""
		}
		return framemetadata.FormatOverlayText(d)

	case "av1":
		tu := splitOBUStream(sample)
		d, ok, _ := framemetadata.ExtractFromAV1TU(tu)
		if !ok {
			return ""
		}
		return framemetadata.FormatOverlayText(d)
	}
	return ""
}

func splitNALUs(sample []byte) [][]byte {
	// Handle both Annex-B and MP4 length-prefixed.
	if hasStartCode(sample) {
		return splitAnnexB(sample)
	}

	var out [][]byte
	for i := 0; i+4 <= len(sample); {
		n := int(uint32(sample[i])<<24 | uint32(sample[i+1])<<16 | uint32(sample[i+2])<<8 | uint32(sample[i+3]))
		i += 4
		if n <= 0 || i+n > len(sample) {
			return nil
		}
		out = append(out, sample[i:i+n])
		i += n
	}
	return out
}

func hasStartCode(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if i+4 <= len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 1 {
			return true
		}
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			return true
		}
	}
	return false
}

func splitAnnexB(b []byte) [][]byte {
	var out [][]byte
	i := 0
	for {
		scPos, scLen := findStartCode(b, i)
		if scPos < 0 {
			break
		}
		j := scPos + scLen
		nextPos, _ := findStartCode(b, j)
		if nextPos < 0 {
			nextPos = len(b)
		}
		if j < nextPos {
			nalu := b[j:nextPos]
			if len(nalu) > 0 {
				out = append(out, nalu)
			}
		}
		i = nextPos
	}
	return out
}

func findStartCode(b []byte, start int) (pos int, length int) {
	for i := start; i+3 <= len(b); i++ {
		if i+4 <= len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 1 {
			return i, 4
		}
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			return i, 3
		}
	}
	return -1, 0
}

func splitOBUStream(b []byte) [][]byte {
	// Best-effort splitter for OBU streams with size fields (typical).
	var out [][]byte
	for i := 0; i < len(b); {
		if i+2 > len(b) {
			return nil
		}
		h := b[i]
		ext := (h & 0x04) != 0
		hasSize := (h & 0x02) != 0
		j := i + 1
		if ext {
			j++
			if j > len(b) {
				return nil
			}
		}
		if !hasSize {
			return nil
		}
		n, sz, ok := decodeLEB128(b[j:])
		if !ok {
			return nil
		}
		j += n
		if j+int(sz) > len(b) {
			return nil
		}
		out = append(out, b[i:j+int(sz)])
		i = j + int(sz)
	}
	return out
}

func decodeLEB128(b []byte) (n int, v uint64, ok bool) {
	var shift uint
	for i := 0; i < len(b) && i < 10; i++ {
		by := b[i]
		v |= uint64(by&0x7F) << shift
		n++
		if (by & 0x80) == 0 {
			return n, v, true
		}
		shift += 7
	}
	return 0, 0, false
}

func ticksToAssTime(ticks int64, timeScale uint32) string {
	if timeScale == 0 {
		return "0:00:00.00"
	}
	// convert to centiseconds
	cs := (ticks * 100) / int64(timeScale)
	cs = max(cs, 0)
	h := cs / (3600 * 100)
	cs -= h * 3600 * 100
	m := cs / (60 * 100)
	cs -= m * 60 * 100
	s := cs / 100
	cs -= s * 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

func assEscape(s string) string {
	// ASS uses \N for newlines; also escape backslashes and braces.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "{", `\{`)
	s = strings.ReplaceAll(s, "}", `\}`)
	s = strings.ReplaceAll(s, "\n", `\N`)
	return s
}

func escapeFilterPath(p string) string {
	// ffmpeg filter args treat ':' as separator; escape it.
	p = filepath.Clean(p)
	p = strings.ReplaceAll(p, `\`, `\\`)
	p = strings.ReplaceAll(p, ":", `\:`)
	return p
}
