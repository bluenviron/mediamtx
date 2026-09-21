package rtsp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testClockRate = 90000
	testFrame     = 3000
)

type testPacket struct {
	raw      int64
	newFrame bool
}

func frames(raws ...int64) []testPacket {
	packets := make([]testPacket, len(raws))
	for i, raw := range raws {
		packets[i] = testPacket{raw: raw, newFrame: true}
	}
	return packets
}

func sanitizeAll(s *ptsSanitizer, packets []testPacket) []int64 {
	out := make([]int64, len(packets))
	for i, p := range packets {
		out[i] = s.sanitize(p.raw, p.newFrame)
	}
	return out
}

func requireMonotonic(t *testing.T, out []int64) {
	for i := 1; i < len(out); i++ {
		require.GreaterOrEqual(t, out[i], out[i-1], "packet %d went backwards", i)
	}
}

func TestPTSSanitizerKeepsRegularTimeline(t *testing.T) {
	s := newPTSSanitizer(testClockRate)

	require.Equal(t, []int64{0, 3000, 6000, 9000, 12000}, sanitizeAll(s, frames(0, 3000, 6000, 9000, 12000)))
}

func TestPTSSanitizerKeepsPacketsOfOneFrameTogether(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	packets := []testPacket{{0, false}, {0, false}, {3000, true}, {3000, false}, {3000, false}}

	require.Equal(t, []int64{0, 0, 3000, 3000, 3000}, sanitizeAll(s, packets))
}

func TestPTSSanitizerSeparatesFramesSharingATimestamp(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	packets := []testPacket{{0, false}, {3000, true}, {3000, false}, {3000, true}, {3000, false}, {6000, true}}

	require.Equal(t, []int64{0, 3000, 3000, 3001, 3001, 6001}, sanitizeAll(s, packets))
}

func TestPTSSanitizerRestampsFrameFromThePast(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	past := int64(-9842 * testClockRate)
	packets := append(frames(0, 3000, 6000), testPacket{past, true}, testPacket{past, false})
	packets = append(packets, frames(9000, 12000)...)

	out := sanitizeAll(s, packets)

	requireMonotonic(t, out)
	require.Equal(t, []int64{0, 3000, 6000, 6001, 6001, 9000, 12000}, out)
}

func TestPTSSanitizerRestampsFrameFromTheFuture(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	future := int64(37600 * testClockRate)

	out := sanitizeAll(s, frames(0, 3000, 6000, future, 9000, 12000))

	requireMonotonic(t, out)
	require.Equal(t, []int64{0, 3000, 6000, 6001, 9000, 12000}, out)
}

func TestPTSSanitizerSurvivesRepeatedIntruders(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	var raws []int64
	for i := range int64(300) {
		raws = append(raws, i*testFrame)
		if i%60 == 30 {
			raws = append(raws, i*testFrame-9842*testClockRate)
		}
	}

	out := sanitizeAll(s, frames(raws...))

	requireMonotonic(t, out)
	require.Equal(t, int64(299*testFrame), out[len(out)-1])
}

func TestPTSSanitizerJoinsPersistentJump(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	jump := int64(-4 * 3600 * testClockRate)

	out := sanitizeAll(s, frames(0, 3000, 6000, jump, jump+3000, jump+6000, jump+9000, jump+12000))

	requireMonotonic(t, out)
	require.Equal(t, []int64{0, 3000, 6000, 6001, 6002, 9000, 12000, 15000}, out)
}

func TestPTSSanitizerJoinsWhenStreamStartsOnAnOutlier(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	future := int64(37600 * testClockRate)

	out := sanitizeAll(s, frames(future, 0, 3000, 6000, 9000))

	requireMonotonic(t, out)
	require.Equal(t, []int64{future, future + 1, future + 2, future + 3, future + 3003}, out)
}

func TestPTSSanitizerKeepsShortReorderingAndGaps(t *testing.T) {
	s := newPTSSanitizer(testClockRate)
	raws := []int64{0, 9000, 3000, 6000, 18000, 4*testClockRate + 18000}

	require.Equal(t, raws, sanitizeAll(s, frames(raws...)))
}
