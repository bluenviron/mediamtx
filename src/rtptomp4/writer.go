package rtptomp4

import (
	"fmt"
	"os"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/src/formatprocessor"
	"github.com/bluenviron/mediamtx/src/logger"
	"github.com/bluenviron/mediamtx/src/unit"
)

type track struct {
	initTrack *fmp4.InitTrack
	nextID    int
}

// MP4Writer writes RTP packets to an MP4 file.
type MP4Writer struct {
	outputPath string
	format     format.Format
	processor  formatprocessor.Processor
	file       *os.File
	track      *track
	mdat       []byte
}

// NewMP4Writer creates a new MP4Writer.
func NewMP4Writer(outputPath string, format format.Format) (*MP4Writer, error) {
	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	// Initialize the format processor
	log, err := logger.New(logger.Info, nil, "", "")
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	processor, err := formatprocessor.New(1500, format, false, log)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create format processor: %w", err)
	}

	// Create track
	track := &track{
		initTrack: &fmp4.InitTrack{
			TimeScale: uint32(format.ClockRate()),
			ID:        1,
		},
	}

	// Initialize track codec based on format type
	switch format := format.(type) {
	case *rtspformat.H264:
		track.initTrack.Codec = &fmp4.CodecH264{
			SPS: format.SPS,
			PPS: format.PPS,
		}
	// Add other format types as needed
	default:
		file.Close()
		return nil, fmt.Errorf("unsupported format type: %T", format)
	}

	return &MP4Writer{
		outputPath: outputPath,
		format:     format,
		processor:  processor,
		file:       file,
		track:      track,
		mdat:       make([]byte, 0),
	}, nil
}

// WriteRTP writes an RTP packet to the MP4 file.
func (w *MP4Writer) WriteRTP(pkt *rtp.Packet) error {
	// Process the RTP packet into a unit
	u, err := w.processor.ProcessRTPPacket(pkt, time.Now(), 0, false)
	if err != nil {
		return fmt.Errorf("failed to process RTP packet: %w", err)
	}

	if u == nil {
		return nil // Skip empty units
	}

	// Convert the unit into an fMP4 sample based on format type
	var sampl fmp4.PartSample

	switch u := u.(type) {
	case *unit.H264:
		err = sampl.FillH264(0, u.AU) // Use 0 as duration, it will be updated later
	// Add other unit types as needed
	default:
		return fmt.Errorf("unsupported unit type: %T", u)
	}

	if err != nil {
		return fmt.Errorf("failed to fill fMP4 sample: %w", err)
	}

	// Append the sample to the mdat box
	w.mdat = append(w.mdat, sampl.Payload...)

	return nil
}

// Close closes the MP4Writer and finalizes the MP4 file.
func (w *MP4Writer) Close() error {
	// Write the init segment
	init := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{w.track.initTrack},
	}

	var buf seekablebuffer.Buffer
	err := init.Marshal(&buf)
	if err != nil {
		return fmt.Errorf("failed to write init segment: %w", err)
	}

	_, err = w.file.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write init segment: %w", err)
	}

	// Write the mdat box
	_, err = w.file.Write(w.mdat)
	if err != nil {
		return fmt.Errorf("failed to write mdat box: %w", err)
	}

	return w.file.Close()
}
