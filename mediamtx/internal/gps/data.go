// Package gps provides GPS data management functionality.
package gps

import (
	"fmt"
	"time"
)

// Data represents GPS information from the database.
type Data struct {
	Timestamp time.Time
	Latitude  float64
	Longitude float64
	Status    string
}

// FormatCoordinate formats latitude or longitude to the required DD.DDDDDD format.
func FormatCoordinate(coord float64) string {
	return fmt.Sprintf("%.6f", coord)
}