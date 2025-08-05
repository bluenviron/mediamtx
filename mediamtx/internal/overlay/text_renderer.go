package overlay

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gocv.io/x/gocv"

	"github.com/bluenviron/mediamtx/internal/gps"
)

// TextRenderer handles OpenCV-based text rendering on video frames.
type TextRenderer struct {
	config    *Config
	fontFace  gocv.HersheyFont
	fontScale float64
	thickness int
	textColor color.RGBA
	bgColor   color.RGBA
	position  OverlayPosition
}

// OverlayPosition defines where to place the overlay text.
type OverlayPosition int

const (
	TopLeft OverlayPosition = iota
	TopRight
	BottomLeft
	BottomRight
)

// NewTextRenderer creates a new text renderer.
func NewTextRenderer(config *Config) (*TextRenderer, error) {
	renderer := &TextRenderer{
		config:    config,
		fontFace:  gocv.FontHersheyComplex,
		fontScale: float64(config.FontSize) / 30.0, // Scale factor based on font size
		thickness: 2,
	}

	// Parse text color
	textColor, err := parseColor(config.TextColor)
	if err != nil {
		return nil, fmt.Errorf("invalid text color: %w", err)
	}
	renderer.textColor = textColor

	// Parse background color
	bgColor, err := parseColor(config.BackgroundColor)
	if err != nil {
		return nil, fmt.Errorf("invalid background color: %w", err)
	}
	renderer.bgColor = bgColor

	// Parse position
	position, err := parsePosition(config.Position)
	if err != nil {
		return nil, fmt.Errorf("invalid position: %w", err)
	}
	renderer.position = position

	return renderer, nil
}

// RenderOverlay renders GPS and ship information onto a video frame.
func (tr *TextRenderer) RenderOverlay(frame gocv.Mat, gpsData *gps.Data, shipName string) error {
	if frame.Empty() {
		return fmt.Errorf("empty frame provided")
	}

	// Format the overlay text - use current local time instead of GPS timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	latStr := gps.FormatCoordinate(gpsData.Latitude)
	lonStr := gps.FormatCoordinate(gpsData.Longitude)

	lines := []string{
		timestamp,
		fmt.Sprintf("%s, %s", latStr, lonStr),
		shipName,
	}

	// Calculate text size and position
	frameSize := frame.Size()
	textSizes := make([]image.Point, len(lines))
	maxWidth := 0
	totalHeight := 0
	lineSpacing := int(float64(tr.config.FontSize) * 1.2)

	for i, line := range lines {
		textSize := gocv.GetTextSize(line, tr.fontFace, tr.fontScale, tr.thickness)
		textSizes[i] = textSize
		if textSize.X > maxWidth {
			maxWidth = textSize.X
		}
		totalHeight += lineSpacing
	}

	// Calculate starting position based on overlay position
	margin := 10
	var startX, startY int

	switch tr.position {
	case TopLeft:
		startX = margin
		startY = margin + lineSpacing
	case TopRight:
		startX = frameSize[1] - maxWidth - margin
		startY = margin + lineSpacing
	case BottomLeft:
		startX = margin
		startY = frameSize[0] - totalHeight - margin
	case BottomRight:
		startX = frameSize[1] - maxWidth - margin
		startY = frameSize[0] - totalHeight - margin
	}

	// Draw background rectangle if background color has transparency
	if tr.bgColor.A > 0 {
		bgRect := image.Rect(
			startX-5,
			startY-lineSpacing+5,
			startX+maxWidth+5,
			startY+totalHeight-lineSpacing+5,
		)
		gocv.Rectangle(&frame, bgRect, tr.bgColor, -1)
	}

	// Draw text lines
	currentY := startY
	for _, line := range lines {
		textPoint := image.Point{X: startX, Y: currentY}
		gocv.PutText(&frame, line, textPoint, tr.fontFace, tr.fontScale, tr.textColor, tr.thickness)
		currentY += lineSpacing
	}

	return nil
}

// parseColor parses color string in "R,G,B" or "R,G,B,A" format.
func parseColor(colorStr string) (color.RGBA, error) {
	parts := strings.Split(colorStr, ",")
	if len(parts) < 3 || len(parts) > 4 {
		return color.RGBA{}, fmt.Errorf("color must be in format 'R,G,B' or 'R,G,B,A'")
	}

	var r, g, b, a uint8

	if r8, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 8); err == nil {
		r = uint8(r8)
	} else {
		return color.RGBA{}, fmt.Errorf("invalid red component: %w", err)
	}

	if g8, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 8); err == nil {
		g = uint8(g8)
	} else {
		return color.RGBA{}, fmt.Errorf("invalid green component: %w", err)
	}

	if b8, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 8); err == nil {
		b = uint8(b8)
	} else {
		return color.RGBA{}, fmt.Errorf("invalid blue component: %w", err)
	}

	a = 255 // Default alpha
	if len(parts) == 4 {
		if a8, err := strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 8); err == nil {
			a = uint8(a8)
		} else {
			return color.RGBA{}, fmt.Errorf("invalid alpha component: %w", err)
		}
	}

	return color.RGBA{R: r, G: g, B: b, A: a}, nil
}

// parsePosition parses position string.
func parsePosition(posStr string) (OverlayPosition, error) {
	switch strings.ToLower(strings.TrimSpace(posStr)) {
	case "top-left":
		return TopLeft, nil
	case "top-right":
		return TopRight, nil
	case "bottom-left":
		return BottomLeft, nil
	case "bottom-right":
		return BottomRight, nil
	default:
		return TopLeft, fmt.Errorf("unknown position '%s', must be one of: top-left, top-right, bottom-left, bottom-right", posStr)
	}
}
