package stream

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// SilenceGenerator generates silence for audio tracks when the original publisher disconnects.
type SilenceGenerator struct {
	ctx           context.Context
	ctxCancel     func()
	stream        *Stream
	desc          *description.Session
	logger        logger.Writer
	running       int32  // atomic: 0=stopped, 1=running
	ticker        *time.Ticker
	processors    map[*description.Media]formatprocessor.Processor
	lastTimestamp atomic.Value // time.Time
	startTime     time.Time
	done          chan struct{}
}

// NewSilenceGenerator creates a new silence generator.
func NewSilenceGenerator(stream *Stream, desc *description.Session, logger logger.Writer) *SilenceGenerator {
	ctx, ctxCancel := context.WithCancel(context.Background())
	
	sg := &SilenceGenerator{
		ctx:        ctx,
		ctxCancel:  ctxCancel,
		stream:     stream,
		desc:       desc,
		logger:     logger,
		processors: make(map[*description.Media]formatprocessor.Processor),
		done:       make(chan struct{}),
	}
	
	// Initialize format processors for each audio media
	for _, media := range desc.Medias {
		if media.Type == description.MediaTypeAudio {
			processor, err := formatprocessor.New(
				stream.RTPMaxPayloadSize,
				media.Formats[0], // Use first format
				true,             // Generate RTP packets
				logger,
			)
			if err != nil {
				logger.Log(3, "failed to create processor for silence generation: %v", err)
				continue
			}
			
			sg.processors[media] = processor
		}
	}
	
	return sg
}

// Start begins generating silence.
func (sg *SilenceGenerator) Start() {
	if !atomic.CompareAndSwapInt32(&sg.running, 0, 1) {
		return // Already running
	}
	
	sg.startTime = time.Now()
	sg.lastTimestamp.Store(sg.startTime)
	
	// Generate silence at 20ms intervals (typical audio frame duration)
	sg.ticker = time.NewTicker(20 * time.Millisecond)
	
	go sg.run()
	
	sg.logger.Log(2, "silence generator started")
}

// Stop stops generating silence.
func (sg *SilenceGenerator) Stop() {
	if !atomic.CompareAndSwapInt32(&sg.running, 1, 0) {
		return // Already stopped
	}
	
	sg.ticker.Stop()
	sg.ctxCancel()
	
	// Wait for the goroutine to finish
	<-sg.done
	
	sg.logger.Log(2, "silence generator stopped")
}

// IsRunning returns whether the generator is currently running.
func (sg *SilenceGenerator) IsRunning() bool {
	return atomic.LoadInt32(&sg.running) == 1
}

func (sg *SilenceGenerator) run() {
	defer sg.ticker.Stop()
	defer close(sg.done)
	
	for {
		select {
		case <-sg.ctx.Done():
			return
			
		case now := <-sg.ticker.C:
			sg.generateSilenceFrame(now)
		}
	}
}

func (sg *SilenceGenerator) generateSilenceFrame(now time.Time) {
	// Check if we should still be running (lock-free)
	if atomic.LoadInt32(&sg.running) == 0 {
		return
	}
	
	for media, processor := range sg.processors {
		pts := int64(now.Sub(sg.startTime)) * int64(media.Formats[0].ClockRate()) / int64(time.Second)
		
		switch fmt := media.Formats[0].(type) {
		case *format.MPEG4Audio:
			// Generate proper AAC-LC silence frame
			silenceAU := sg.generateProperAACLCSilence(fmt)
			if len(silenceAU) == 0 {
				continue
			}
			
			au := &unit.MPEG4Audio{
				Base: unit.Base{
					RTPPackets: []*rtp.Packet{},
					NTP:        now,
					PTS:        pts,
				},
				AUs: [][]byte{silenceAU},
			}
			
			// Process to generate RTP packets
			err := processor.ProcessUnit(au)
			if err != nil {
				sg.logger.Log(3, "failed to process AAC silence: %v", err)
				continue
			}
			
			// Write to stream safely
			func() {
				defer func() {
					if r := recover(); r != nil {
						sg.logger.Log(3, "recovered from panic: %v", r)
					}
				}()
				sg.stream.WriteUnit(media, media.Formats[0], au)
			}()
			
		case *format.Opus:
			// Opus DTX frame for silence
			au := &unit.Opus{
				Base: unit.Base{
					RTPPackets: []*rtp.Packet{},
					NTP:        now,
					PTS:        pts,
				},
				Packets: [][]byte{{0xF8}}, // DTX frame
			}
			
			err := processor.ProcessUnit(au)
			if err != nil {
				sg.logger.Log(3, "failed to process Opus silence: %v", err)
				continue
			}
			
			func() {
				defer func() {
					if r := recover(); r != nil {
						sg.logger.Log(3, "recovered from panic: %v", r)
					}
				}()
				sg.stream.WriteUnit(media, media.Formats[0], au)
			}()
		}
	}
	
	sg.lastTimestamp.Store(now)
}

// generateOpusSilence creates an Opus silence frame
func (sg *SilenceGenerator) generateOpusSilence() []byte {
	// Opus DTX (discontinuous transmission) frame for silence
	// This is a minimal valid Opus packet that represents silence
	return []byte{0xF8} // Opus DTX frame
}

// generateProperAACLCSilence creates a valid AAC-LC silence frame with proper headers
func (sg *SilenceGenerator) generateProperAACLCSilence(fmt *format.MPEG4Audio) []byte {
	// Create a minimal valid AAC frame with ADTS header for silence
	// This ensures the frame has proper codec parameters
	
	if fmt.Config == nil {
		return nil
	}
	
	// Get sampling frequency index for ADTS header
	samplingFreqIndex := getSamplingFrequencyIndex(fmt.Config.SampleRate)
	if samplingFreqIndex == 0xF {
		// Unsupported sample rate
		return nil
	}
	
	channels := int(fmt.Config.ChannelCount)
	if channels == 0 || channels > 7 {
		channels = 2 // Default to stereo
	}
	
	// Create minimal AAC raw data block (silence)
	// Single Channel Element (SCE) or Channel Pair Element (CPE)
	var rawDataBlock []byte
	if channels == 1 {
		// SCE: ID = 0, element_instance_tag = 0
		rawDataBlock = []byte{0x00, 0x00} // Minimal SCE silence
	} else {
		// CPE: ID = 1, element_instance_tag = 0  
		rawDataBlock = []byte{0x20, 0x00} // Minimal CPE silence
	}
	
	// Add End of frame
	rawDataBlock = append(rawDataBlock, 0x70) // ID_END
	
	// Calculate frame length (ADTS header + raw data)
	frameLength := 7 + len(rawDataBlock) // 7 byte ADTS header
	
	// Build ADTS header
	adts := make([]byte, 7)
	
	// Syncword (12 bits) + ID (1) + Layer (2) + Protection absent (1)
	adts[0] = 0xFF // Syncword part 1
	adts[1] = 0xF1 // Syncword part 2 + ID=0 (MPEG-4) + Layer=00 + Protection=1
	
	// Profile (2) + Sampling freq index (4) + Private (1) + Channels (3 bits of 4)
	adts[2] = byte((1 << 6) | // Profile: AAC LC = 1 (profile - 1)
		(int(samplingFreqIndex) << 2) | // Sampling frequency index
		(0 << 1) | // Private bit
		((channels >> 2) & 0x01)) // Channel config high bit
	
	// Channels (remaining bit) + Original (1) + Home (1) + Copyright ID (1) + Start (1) + Frame length (2 bits of 13)
	adts[3] = byte(((channels & 0x03) << 6) | // Channel config low bits
		(0 << 5) | // Original/copy
		(0 << 4) | // Home
		(0 << 3) | // Copyright ID bit
		(0 << 2) | // Copyright ID start
		((frameLength >> 11) & 0x03)) // Frame length high bits
	
	// Frame length (middle 8 bits)
	adts[4] = byte((frameLength >> 3) & 0xFF)
	
	// Frame length (low 3 bits) + Buffer fullness (5 bits of 11)
	adts[5] = byte(((frameLength & 0x07) << 5) | 0x1F) // Buffer fullness VBR
	
	// Buffer fullness (low 6 bits) + Number of AAC frames (2 bits) 
	adts[6] = 0xFC // Buffer fullness VBR + 1 frame
	
	// Combine ADTS header with raw data block
	return append(adts, rawDataBlock...)
}

// getSamplingFrequencyIndex returns the ADTS sampling frequency index
func getSamplingFrequencyIndex(sampleRate int) byte {
	switch sampleRate {
	case 96000:
		return 0
	case 88200:
		return 1
	case 64000:
		return 2
	case 48000:
		return 3
	case 44100:
		return 4
	case 32000:
		return 5
	case 24000:
		return 6
	case 22050:
		return 7
	case 16000:
		return 8
	case 12000:
		return 9
	case 11025:
		return 10
	case 8000:
		return 11
	case 7350:
		return 12
	default:
		return 0xF // Invalid
	}
}

// generateG711Silence creates a G.711 silence frame
func (sg *SilenceGenerator) generateG711Silence(fmt *format.G711) []byte {
	// G.711 uses different silence values for µ-law and A-law
	var silenceValue byte
	if fmt.MULaw {
		silenceValue = 0xFF // µ-law silence
	} else {
		silenceValue = 0xD5 // A-law silence
	}
	
	// 20ms of samples at 8kHz = 160 samples
	silenceFrame := make([]byte, 160)
	for i := range silenceFrame {
		silenceFrame[i] = silenceValue
	}
	
	return silenceFrame
}