// Package recorder contains the recorder functionality.
package recorder

import (
	"bufio"
	"os"
	"path/filepath"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	passthroughTSMaxBufferSize = 64 * 1024
)

// formatPassthroughTS is a recorder format that passes through MPEG-TS streams directly.
type formatPassthroughTS struct {
	ri *recorderInstance

	dw             *dynamicWriter
	bw             *bufio.Writer
	hasVideo       bool
	currentSegment *formatPassthroughTSSegment
}

// formatPassthroughTSSegment represents a segment of a MPEG-TS recording.
type formatPassthroughTSSegment struct {
	f         *formatPassthroughTS
	startDTS  time.Duration
	startNTP  time.Time
	startTime time.Time // Wall clock time when segment started

	path      string
	fi        *os.File
	lastFlush time.Duration
	lastDTS   time.Duration
}

func (s *formatPassthroughTSSegment) initialize() {
	s.lastFlush = s.startDTS
	s.lastDTS = s.startDTS
	s.startTime = time.Now() // Initialize start time for segment duration tracking
	s.f.dw.setTarget(s)
}

func (s *formatPassthroughTSSegment) close() error {
	err := s.f.bw.Flush()

	if s.fi != nil {
		s.f.ri.Log(logger.Debug, "closing segment %s", s.path)
		err2 := s.fi.Close()
		if err == nil {
			err = err2
		}

		if err2 == nil {
			duration := s.lastDTS - s.startDTS
			s.f.ri.onSegmentComplete(s.path, duration)
		}
	}

	return err
}

func (s *formatPassthroughTSSegment) Write(p []byte) (int, error) {
	s.f.ri.Log(logger.Debug, "writing %d bytes to segment", len(p))
	if s.fi == nil {
		s.path = recordstore.Path{Start: s.startNTP}.Encode(s.f.ri.pathFormat2)
		s.f.ri.Log(logger.Info, "creating segment %s", s.path)

		// Ensure the directory exists
		dir := filepath.Dir(s.path)
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			s.f.ri.Log(logger.Error, "failed to create directory: %v", err)
			return 0, err
		}

		// Create the file
		fi, err := os.Create(s.path)
		if err != nil {
			s.f.ri.Log(logger.Error, "failed to create file: %v", err)
			return 0, err
		}

		s.f.ri.onSegmentCreate(s.path)
		s.fi = fi
	}

	// Write the data
	n, err := s.fi.Write(p)
	if err != nil {
		s.f.ri.Log(logger.Error, "error writing to file: %v", err)
	}
	return n, err
}

func (f *formatPassthroughTS) initialize() bool {
	f.ri.Log(logger.Debug, "initializing MPEG-TS passthrough recorder")
	f.dw = &dynamicWriter{}
	f.bw = bufio.NewWriterSize(f.dw, passthroughTSMaxBufferSize)

	// Check if there are any media formats available
	if len(f.ri.stream.Desc.Medias) == 0 {
		f.ri.Log(logger.Warn, "no media formats available for passthrough recording")
		return false
	}

	// Use the first available media format for the reader
	media := f.ri.stream.Desc.Medias[0]
	var format rtspformat.Format
	if len(media.Formats) > 0 {
		format = media.Formats[0]
		f.ri.Log(logger.Debug, "using format: %s", format.Codec())
	} else {
		f.ri.Log(logger.Debug, "no format available in media")
	}

	// Register a reader for the MPEG-TS raw stream
	f.ri.stream.AddReader(
		f.ri,
		media,
		format,
		func(u unit.Unit) error {
			// Handle Generic units containing MPEG-TS data
			genericUnit, ok := u.(*unit.Generic)
			if !ok {
				f.ri.Log(logger.Debug, "received non-Generic unit: %T", u)
				return nil
			}

			// Extract data from Generic unit's RTPPackets
			if len(genericUnit.RTPPackets) == 0 {
				f.ri.Log(logger.Debug, "received Generic unit with no RTP packets")
				return nil
			}

			// Combine all packet payloads
			var data []byte
			for _, pkt := range genericUnit.RTPPackets {
				data = append(data, pkt.Payload...)
			}

			f.ri.Log(logger.Debug, "received Generic unit: %d RTP packets, %d bytes",
				len(genericUnit.RTPPackets), len(data))

			if len(data) == 0 {
				f.ri.Log(logger.Debug, "received unit with no data")
				return nil
			}

			return f.write(
				time.Duration(genericUnit.PTS),
				genericUnit.NTP,
				true, // Assume video is present
				true, // Assume random access
				func() error {
					_, err := f.bw.Write(data)
					return err
				},
			)
		})

	f.ri.Log(logger.Info, "using MPEG-TS passthrough mode")
	return true
}

func (f *formatPassthroughTS) close() {
	if f.currentSegment != nil {
		f.currentSegment.close() //nolint:errcheck
	}
}

func (f *formatPassthroughTS) write(
	dts time.Duration,
	ntp time.Time,
	isVideo bool,
	randomAccess bool,
	writeCB func() error,
) error {
	f.ri.Log(logger.Debug, "writing MPEGTS data, dts: %v", dts)
	if isVideo {
		f.hasVideo = true
	}

	if f.currentSegment == nil {
		f.ri.Log(logger.Debug, "creating new segment")
		f.currentSegment = &formatPassthroughTSSegment{
			f:        f,
			startDTS: dts,
			startNTP: ntp,
		}
		f.currentSegment.initialize()
		f.dw.setTarget(f.currentSegment)
	}

	err := writeCB()
	if err != nil {
		f.ri.Log(logger.Error, "error writing data: %v", err)
		return err
	}

	err = f.bw.Flush()
	if err != nil {
		f.ri.Log(logger.Error, "error flushing buffer: %v", err)
		return err
	}
	f.ri.Log(logger.Debug, "data written and flushed successfully")

	// Update the lastDTS value
	f.currentSegment.lastDTS = dts

	// Check if segment duration is exceeded using wall clock time instead of DTS
	elapsed := time.Since(f.currentSegment.startTime)
	if elapsed >= f.ri.segmentDuration {
		f.ri.Log(logger.Info, "segment duration reached (%v), closing segment", elapsed)
		f.currentSegment.close()
		f.currentSegment = nil
	}

	return nil
}
