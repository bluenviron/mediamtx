// Package overlay provides video overlay functionality for MediaMTX streams.
package overlay

import "time"

// Config holds the overlay configuration.
type Config struct {
	// Enable overlay functionality
	Enabled bool

	// Database configuration
	DatabaseHost     string
	DatabasePort     int
	DatabaseUser     string
	DatabasePassword string
	DatabaseName     string

	// GPS update interval (default: 1 second)
	UpdateInterval time.Duration

	// Text rendering configuration
	FontPath       string
	FontSize       int
	TextColor      string      // Format: "R,G,B" or "R,G,B,A"
	BackgroundColor string     // Format: "R,G,B,A" for transparency
	Position       string      // "top-left", "top-right", "bottom-left", "bottom-right"
	
	// Performance settings
	MaxConnections int // Database connection pool size
}

// DefaultConfig returns the default overlay configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:         false,
		DatabaseHost:    "localhost",
		DatabasePort:    5432,
		DatabaseUser:    "gpsuser",
		DatabasePassword: "gpspassword", 
		DatabaseName:    "gpsdb",
		UpdateInterval:  time.Second,
		FontPath:        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		FontSize:        24,
		TextColor:       "255,255,255",
		BackgroundColor: "0,0,0,128",
		Position:        "top-left",
		MaxConnections:  5,
	}
}