package mpegts

import (
	"sync"
	"time"
)

const (
	maximum           = 0x1FFFFFFFF // 33 bits
	negativeThreshold = 0x1FFFFFFFF / 2
	clockRate         = 90000
)

// TimeDecoder is a MPEG-TS timestamp decoder.
type TimeDecoder struct {
	initialized bool
	tsOverall   time.Duration
	tsPrev      int64
	mutex       sync.Mutex
}

// NewTimeDecoder allocates a TimeDecoder.
func NewTimeDecoder() *TimeDecoder {
	return &TimeDecoder{}
}

// Decode decodes a MPEG-TS timestamp.
func (d *TimeDecoder) Decode(ts int64) time.Duration {
	d.mutex.Lock()
	defer d.mutex.Unlock()

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
