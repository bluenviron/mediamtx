package recorder

import (
	"fmt"
	"testing"
	"time"
)

func TestRecorderPauseFreshnessByClockMode(t *testing.T) {
	for _, localMode := range []bool{true, false} {
		for _, stall := range []time.Duration{3 * time.Second, 30 * time.Second, 50 * time.Second} {
			for _, lead := range []time.Duration{-1500 * time.Millisecond, 0, 1500 * time.Millisecond} {
				for _, audioFirst := range []bool{true, false} {
					for _, delayedResume := range []bool{false, true} {
						name := fmt.Sprintf(
							"local=%v/stall=%v/lead=%v/audioFirst=%v/delayedResume=%v",
							localMode, stall, lead, audioFirst, delayedResume)
						t.Run(name, func(t *testing.T) {
							runRecorderPauseCase(t, stall, lead, audioFirst, delayedResume, &localMode)
						})
					}
				}
			}
		}
	}
}
