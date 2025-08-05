package overlay

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"sync"
	"time"

	"gocv.io/x/gocv"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"

	"github.com/bluenviron/mediamtx/internal/gps"
)

// Default H.264 parameters (1920x1080 baseline)
var (
	H264DefaultSPS = []byte{
		0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
		0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
	}

	H264DefaultPPS = []byte{0x08, 0x06, 0x07, 0x08}
)

// Engine is the main overlay processing engine.
type Engine struct {
	config          *Config
	gpsDataProvider gps.DataProvider
	textRenderer    *TextRenderer
	enabled         bool
	mu              sync.RWMutex
	shipName        string

	// H.264 processing components
	h264Format *format.H264

	// Frame dimensions (will be updated from SPS)
	width  int
	height int
}

// NewEngine creates a new overlay engine.
func NewEngine(config *Config, gpsDataProvider gps.DataProvider, shipName string) (*Engine, error) {
	if !config.Enabled {
		return &Engine{
			config:  config,
			enabled: false,
		}, nil
	}

	// Initialize text renderer
	textRenderer, err := NewTextRenderer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create text renderer: %w", err)
	}

	// Initialize H.264 format with default parameters
	h264Format := &format.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
		SPS:               []byte{0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20},
		PPS:               []byte{0x08, 0x06, 0x07, 0x08},
	}

	return &Engine{
		config:          config,
		gpsDataProvider: gpsDataProvider,
		textRenderer:    textRenderer,
		enabled:         true,
		shipName:        shipName,
		h264Format:      h264Format,
		width:           1920, // Default width
		height:          1080, // Default height
	}, nil
}

// Close closes the overlay engine and releases resources.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.enabled = false
	return nil
}

// IsEnabled returns whether the overlay is enabled.
func (e *Engine) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// ProcessFrame processes a video frame and applies overlay if enabled.
// H264 Access Units are provided as a slice of byte slices.
func (e *Engine) ProcessFrame(au [][]byte) [][]byte {
	if !e.IsEnabled() {
		return au
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check if we have valid NAL units
	if len(au) == 0 {
		fmt.Println("no NAL units")
		return au
	}

	// Check if this AU contains valid frame data
	hasValidFrame := false
	for _, nalu := range au {
		if len(nalu) > 0 {
			nalType := mch264.NALUType(nalu[0] & 0x1F)
			// Check for slice NAL units (1-5) which contain actual frame data
			if nalType >= 1 && nalType <= 5 {
				hasValidFrame = true
				break
			}
		}
	}

	if !hasValidFrame {
		fmt.Println("No valid frame data found in AU, returning original")
		return au
	}

	// Update format parameters from AU
	e.updateH264Parameters(au)

	// Try to decode and re-encode the frame with overlay
	processedAU, err := e.processH264FrameWithOverlay(au)
	if err != nil {
		// If processing fails, return original AU
		fmt.Printf("Frame processing failed: %v, returning original AU\n", err)
		return au
	}

	return processedAU
}

// processH264FrameWithOverlay decodes H.264, applies overlay, and re-encodes
func (e *Engine) processH264FrameWithOverlay(au [][]byte) ([][]byte, error) {
	// Use proper H.264 processing approach instead of FFmpeg
	processedAU, err := e.processH264Directly(au)
	if err != nil {
		fmt.Printf("Direct H.264 processing failed: %v\n", err)
		return au, err
	}

	return processedAU, nil
}

// processH264Directly processes H.264 AU directly without FFmpeg
func (e *Engine) processH264Directly(au [][]byte) ([][]byte, error) {
	fmt.Printf("Processing H.264 AU with %d NAL units\n", len(au))

	// Extract and validate SPS/PPS
	sps := e.h264Format.SPS
	pps := e.h264Format.PPS
	paramsChanged := false

	// Use default parameters if not available (like in format_fmp4.go)
	if sps == nil {
		sps = H264DefaultSPS
		fmt.Printf("Using default SPS\n")
	}
	if pps == nil {
		pps = H264DefaultPPS
		fmt.Printf("Using default PPS\n")
	}

	// Check if this AU contains IDR frame (key frame) for overlay
	hasIDR := false
	for i, nalu := range au {
		if len(nalu) > 0 {
			typ := mch264.NALUType(nalu[0] & 0x1F)
			fmt.Printf("NAL %d: type=%d, size=%d\n", i, typ, len(nalu))
			if typ == mch264.NALUTypeIDR {
				hasIDR = true
				fmt.Printf("Found IDR frame at NAL %d\n", i)
			}
		}
	}

	// Only apply overlay on IDR frames for now
	if !hasIDR {
		// For non-IDR frames, just pass through
		fmt.Printf("Non-IDR frame, passing through\n")
		return au, nil
	}

	fmt.Printf("IDR frame detected, applying overlay...\n")

	// Decode frame, apply overlay, and re-encode
	processedAU, err := e.decodeApplyOverlayAndEncode(au)
	if err != nil {
		fmt.Printf("Overlay processing failed: %v, returning original AU\n", err)
		return au, err
	}

	// Update format parameters if changed
	if paramsChanged {
		e.h264Format.SafeSetParams(sps, pps)
		fmt.Printf("Updated H.264 parameters\n")
	}

	fmt.Printf("Successfully applied overlay to IDR frame, returning %d NAL units\n", len(processedAU))
	return processedAU, nil
}

// decodeApplyOverlayAndEncode decodes H.264 frame, applies overlay, and re-encodes
func (e *Engine) decodeApplyOverlayAndEncode(au [][]byte) ([][]byte, error) {
	// Step 1: Decode H.264 to raw frame
	rawFrame, err := e.decodeH264ToMat(au)
	if err != nil {
		return nil, fmt.Errorf("failed to decode H.264: %w", err)
	}
	defer rawFrame.Close()

	// Step 2: Apply overlay using TextRenderer
	err = e.applyOverlay(rawFrame)
	if err != nil {
		return nil, fmt.Errorf("failed to apply overlay: %w", err)
	}

	// Step 3: Re-encode to H.264
	encodedAU, err := e.encodeMatToH264(rawFrame)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode H.264: %w", err)
	}

	return encodedAU, nil
}

// applyOverlay applies overlay using the TextRenderer
func (e *Engine) applyOverlay(frame *gocv.Mat) error {
	if e.textRenderer == nil {
		// Fallback to simple "Hello World" if TextRenderer is not available
		return e.applyHelloWorldOverlay(frame)
	}

	// Get current GPS data
	var gpsData *gps.Data
	if e.gpsDataProvider != nil {
		gpsData = e.gpsDataProvider.GetCurrentGPS()
	} else {
		// Create dummy GPS data if provider is not available
		gpsData = &gps.Data{
			Latitude:  37.5665, // Seoul coordinates
			Longitude: 126.9780,
			Timestamp: time.Now(),
		}
	}

	// Apply overlay using TextRenderer
	err := e.textRenderer.RenderOverlay(*frame, gpsData, e.shipName)
	if err != nil {
		fmt.Printf("TextRenderer overlay failed: %v, falling back to simple overlay\n", err)
		return e.applyHelloWorldOverlay(frame)
	}

	fmt.Printf("Successfully applied TextRenderer overlay\n")
	return nil
}

// decodeH264ToMat decodes H.264 AU to OpenCV Mat
func (e *Engine) decodeH264ToMat(au [][]byte) (*gocv.Mat, error) {
	fmt.Printf("Decoding H.264 to Mat (frame size: %dx%d)\n", e.width, e.height)

	// Write AU to temporary file
	tempFile, err := e.writeAUToTempFile(au)
	if err != nil {
		return nil, fmt.Errorf("failed to write AU to temp file: %w", err)
	}
	defer os.Remove(tempFile)

	// Use FFmpeg to decode to raw BGR data
	cmd := exec.Command("ffmpeg",
		"-i", tempFile,
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"-s", fmt.Sprintf("%dx%d", e.width, e.height),
		"-v", "error",
		"-")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("FFmpeg decode failed: %w (stderr: %s)", err, stderr.String())
	}

	// Convert raw data to OpenCV Mat
	rawData := stdout.Bytes()
	expectedSize := e.width * e.height * 3

	fmt.Printf("FFmpeg decode successful, raw data size: %d bytes\n", len(rawData))

	if len(rawData) != expectedSize {
		return nil, fmt.Errorf("unexpected raw data size: got %d, expected %d", len(rawData), expectedSize)
	}

	frame, err := gocv.NewMatFromBytes(e.height, e.width, gocv.MatTypeCV8UC3, rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mat: %w", err)
	}

	fmt.Printf("Successfully created OpenCV Mat: %dx%d\n", frame.Cols(), frame.Rows())
	return &frame, nil
}

// applyHelloWorldOverlay adds "Hello World" text to the frame
func (e *Engine) applyHelloWorldOverlay(frame *gocv.Mat) error {
	// Create text properties
	text := "Hello World"
	point := image.Point{X: 50, Y: 100}
	whiteColor := color.RGBA{R: 255, G: 255, B: 255, A: 255} // White color
	thickness := 2
	scale := 2.0

	// Add text to frame
	fmt.Println("applyHelloWorldOverlay")
	gocv.PutText(frame, text, point, gocv.FontHersheyComplex, scale, whiteColor, thickness)

	return nil
}

// encodeMatToH264 encodes OpenCV Mat back to H.264 AU
func (e *Engine) encodeMatToH264(frame *gocv.Mat) ([][]byte, error) {
	fmt.Printf("Encoding Mat to H.264 (frame size: %dx%d)\n", frame.Cols(), frame.Rows())

	// Convert Mat to raw BGR data
	rawData := frame.ToBytes()
	fmt.Printf("Converted Mat to raw data: %d bytes\n", len(rawData))

	// Use FFmpeg to encode raw BGR to H.264
	cmd := exec.Command("ffmpeg",
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"-s", fmt.Sprintf("%dx%d", frame.Cols(), frame.Rows()),
		"-i", "-",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-f", "h264",
		"-v", "error",
		"-")

	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdin = &stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Write raw data to stdin
	_, err := stdin.Write(rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to write raw data: %w", err)
	}

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("FFmpeg encode failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse encoded data into NAL units
	encodedData := stdout.Bytes()
	nalUnits := e.parseH264NALUnits(encodedData)

	fmt.Printf("FFmpeg encode successful, created %d NAL units\n", len(nalUnits))
	return nalUnits, nil
}

// writeAUToTempFile writes H.264 AU data to a temporary file
func (e *Engine) writeAUToTempFile(au [][]byte) (string, error) {
	if len(au) == 0 {
		return "", fmt.Errorf("no NAL units provided")
	}

	// Create temporary file
	tempFile, err := os.CreateTemp("", "h264_au_*.h264")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// Write NAL units with proper start codes
	for i, nalu := range au {
		if len(nalu) == 0 {
			continue
		}

		// Write start code (0x00 0x00 0x00 0x01)
		_, err := tempFile.Write([]byte{0x00, 0x00, 0x00, 0x01})
		if err != nil {
			return "", fmt.Errorf("failed to write start code: %w", err)
		}

		// Write NAL unit data
		_, err = tempFile.Write(nalu)
		if err != nil {
			return "", fmt.Errorf("failed to write NAL unit %d: %w", i, err)
		}
	}

	// Ensure all data is written to disk
	err = tempFile.Sync()
	if err != nil {
		return "", fmt.Errorf("failed to sync temp file: %w", err)
	}

	return tempFile.Name(), nil
}

// parseH264NALUnits parses H.264 encoded data into NAL units
func (e *Engine) parseH264NALUnits(data []byte) [][]byte {
	var nalUnits [][]byte

	// Parse NAL units by looking for start codes (0x00 0x00 0x00 0x01)
	start := 0
	for i := 0; i < len(data)-3; i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x00 && data[i+3] == 0x01 {
			if start < i {
				nalUnits = append(nalUnits, data[start:i])
			}
			start = i + 4
		}
	}

	// Add remaining data
	if start < len(data) {
		nalUnits = append(nalUnits, data[start:])
	}

	return nalUnits
}

// updateH264Parameters updates H.264 format parameters from AU
func (e *Engine) updateH264Parameters(au [][]byte) {
	sps := e.h264Format.SPS
	pps := e.h264Format.PPS
	update := false

	for _, nalu := range au {
		if len(nalu) == 0 {
			continue
		}

		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS:
			if !bytes.Equal(nalu, sps) {
				sps = nalu
				update = true
				fmt.Printf("Updated SPS in parameters\n")
			}

		case mch264.NALUTypePPS:
			if !bytes.Equal(nalu, pps) {
				pps = nalu
				update = true
				fmt.Printf("Updated PPS in parameters\n")
			}
		}
	}

	if update {
		e.h264Format.SafeSetParams(sps, pps)
	}
}

// updateFrameDimensions extracts frame dimensions from SPS
func (e *Engine) updateFrameDimensions(sps []byte) {
	// For now, we'll use default dimensions
	// In a full implementation, you would parse the SPS to extract width/height
	fmt.Printf("SPS received, using default dimensions\n")
}

// ProcessMatFrame processes an OpenCV Mat frame directly (for testing or when raw frames are available).
func (e *Engine) ProcessMatFrame(frame gocv.Mat) error {
	if !e.IsEnabled() {
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Get current GPS data from provider
	gpsData := e.gpsDataProvider.GetCurrentGPS()

	// Apply overlay
	return e.textRenderer.RenderOverlay(frame, gpsData, e.shipName)
}

// UpdateConfig updates the engine configuration.
func (e *Engine) UpdateConfig(newConfig *Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// If enabling/disabling overlay
	if e.enabled != newConfig.Enabled {
		if newConfig.Enabled {
			// Need to reinitialize components
			return fmt.Errorf("enabling overlay requires engine restart")
		} else {
			// Disabling - close resources
			e.textRenderer = nil
			e.enabled = false
		}
	}

	e.config = newConfig
	return nil
}

// GetGPSData returns the current GPS data (for debugging/monitoring).
func (e *Engine) GetGPSData() *gps.Data {
	if !e.IsEnabled() || e.gpsDataProvider == nil {
		return nil
	}

	return e.gpsDataProvider.GetCurrentGPS()
}
