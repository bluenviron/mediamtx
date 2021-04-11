package h264

import (
	"time"
)

// DTSEstimator is a DTS estimator.
type DTSEstimator struct {
	initializing int
	prevDTS      time.Duration
	prevPTS      time.Duration
	prevPrevPTS  time.Duration
}

// NewDTSEstimator allocates a DTSEstimator.
func NewDTSEstimator() *DTSEstimator {
	return &DTSEstimator{
		initializing: 2,
	}
}

// Feed provides PTS to the estimator, and returns the estimated DTS.
func (d *DTSEstimator) Feed(pts time.Duration) time.Duration {
	if d.initializing > 0 {
		d.initializing--
		dts := d.prevDTS + time.Millisecond
		d.prevPrevPTS = d.prevPTS
		d.prevPTS = pts
		d.prevDTS = dts
		return dts
	}

	dts := func() time.Duration {
		// P or I frame
		if pts > d.prevPTS {
			// previous frame was B
			// use the DTS of the previous frame
			if d.prevPTS < d.prevPrevPTS {
				return d.prevPTS
			}

			// previous frame was P or I
			// use two frames ago plus a small quantity
			// to avoid non-monotonous DTS with B-frames
			return d.prevPrevPTS + time.Millisecond
		}

		// B Frame
		// do not increase
		return d.prevDTS + time.Millisecond
	}()

	d.prevPrevPTS = d.prevPTS
	d.prevPTS = pts
	d.prevDTS = dts

	return dts
}
