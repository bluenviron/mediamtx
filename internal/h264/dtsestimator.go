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
	switch d.initializing {
	case 2:
		d.initializing--
		return 0

	case 1:
		d.initializing--
		d.prevPTS = pts
		d.prevDTS = time.Millisecond
		return time.Millisecond
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
		// increase by a small quantity
		return d.prevDTS + time.Millisecond
	}()

	d.prevPrevPTS = d.prevPTS
	d.prevPTS = pts
	d.prevDTS = dts

	return dts
}
