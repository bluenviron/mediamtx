// Package mpegtstimedec contains a MPEG-TS timestamp decoder.
package mpegtstimedec

import (
	"time"
)

const (
	maximum           = 0x1FFFFFFFF // 33 bits
	negativeThreshold = 0xFFFFFFF
	clockRate         = 90000
)

// Decoder is a MPEG-TS timestamp decoder.
type Decoder struct {
	initialized bool
	tsOverall   time.Duration
	tsPrev      int64
}

// New allocates a Decoder.
func New() *Decoder {
	return &Decoder{}
}

// Decode decodes a MPEG-TS timestamp.
func (d *Decoder) Decode(ts int64) time.Duration {
	if !d.initialized {
		d.initialized = true
		d.tsPrev = ts
		return 0
	}

	diff := (ts - d.tsPrev) & maximum

	// negative difference
	if diff > negativeThreshold {
		diff = (d.tsPrev - ts) & maximum
		d.tsPrev = ts
		d.tsOverall -= time.Duration(diff)
	} else {
		d.tsPrev = ts
		d.tsOverall += time.Duration(diff)
	}

	// avoid an int64 overflow and preserve resolution by splitting division into two parts:
	// first add the integer part, then the decimal part.
	secs := d.tsOverall / clockRate
	dec := d.tsOverall % clockRate
	return secs*time.Second + dec*time.Second/clockRate
}
