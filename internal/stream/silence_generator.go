package stream

import (
	"context"
	"sync"
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
	mutex         sync.RWMutex
	running       bool
	ticker        *time.Ticker
	processors    map[*description.Media]formatprocessor.Processor
	lastTimestamp time.Time
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
	sg.mutex.Lock()
	defer sg.mutex.Unlock()
	
	if sg.running {
		return
	}
	
	sg.running = true
	sg.lastTimestamp = time.Now()
	
	// Generate silence at 20ms intervals (typical audio frame duration)
	sg.ticker = time.NewTicker(20 * time.Millisecond)
	
	go sg.run()
	
	sg.logger.Log(2, "silence generator started")
}

// Stop stops generating silence.
func (sg *SilenceGenerator) Stop() {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()
	
	if !sg.running {
		return
	}
	
	sg.running = false
	sg.ticker.Stop()
	sg.ctxCancel()
	
	sg.logger.Log(2, "silence generator stopped")
}

// IsRunning returns whether the generator is currently running.
func (sg *SilenceGenerator) IsRunning() bool {
	sg.mutex.RLock()
	defer sg.mutex.RUnlock()
	return sg.running
}

func (sg *SilenceGenerator) run() {
	defer sg.ticker.Stop()
	
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
	for media, processor := range sg.processors {
		// Create silence based on the audio format
		var silenceData []byte
		var err error
		
		switch fmt := media.Formats[0].(type) {
		case *format.Opus:
			// Opus silence frame (5ms of silence at 48kHz)
			silenceData = sg.generateOpusSilence()
			
		case *format.MPEG4Audio:
			// AAC silence frame
			silenceData = sg.generateAACLCSilence(fmt)
			
		case *format.G711:
			// G.711 silence frame
			silenceData = sg.generateG711Silence(fmt)
			
		default:
			// Generic silence - just empty data
			silenceData = make([]byte, 160) // 20ms at 8kHz
		}
		
		if len(silenceData) == 0 {
			continue
		}
		
		// Create unit with silence data
		au := &unit.Generic{
			Base: unit.Base{
				RTPPackets: []*rtp.Packet{},
				NTP:        now,
			},
		}
		
		// Process through format processor to generate RTP packets
		err = processor.ProcessUnit(au)
		if err != nil {
			sg.logger.Log(3, "failed to process silence unit: %v", err)
			continue
		}
		
		// Write to stream
		sg.stream.WriteUnit(media, media.Formats[0], au)
	}
	
	sg.lastTimestamp = now
}

// generateOpusSilence creates an Opus silence frame
func (sg *SilenceGenerator) generateOpusSilence() []byte {
	// Opus DTX (discontinuous transmission) frame for silence
	// This is a minimal valid Opus packet that represents silence
	return []byte{0xF8} // Opus DTX frame
}

// generateAACLCSilence creates an AAC-LC silence frame
func (sg *SilenceGenerator) generateAACLCSilence(fmt *format.MPEG4Audio) []byte {
	// AAC silence frame - zeros representing silence
	// Frame size depends on sample rate and channel configuration
	frameSize := 1024 // Default AAC frame size
	if fmt.Config.SampleRate <= 24000 {
		frameSize = 512
	}
	
	channels := fmt.Config.ChannelCount
	if channels == 0 {
		channels = 2 // Default to stereo
	}
	
	// Generate silence samples (16-bit PCM worth of zeros, but this would be encoded)
	// For simplicity, we'll create a minimal valid AAC frame
	// In a real implementation, you'd want to create a proper AAC encoder
	silenceFrame := make([]byte, frameSize*channels/8) // Rough estimate
	return silenceFrame
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