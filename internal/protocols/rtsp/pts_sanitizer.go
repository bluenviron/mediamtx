package rtsp

// ptsSanitizerJoinAfter is the number of consecutive frames on a new timeline
// after which that timeline is joined to the previous one.
const ptsSanitizerJoinAfter = 3

// ptsSanitizer keeps a track's timeline continuous when a source stamps a
// frame far away from its neighbours. Some DVRs send, every couple of seconds,
// a frame stamped hours in the past or in the future. The DTS extractors reject
// any step backwards, and a single rejection terminates the HLS muxer (HTTP 500
// until a new one is created) and restarts the recording segment. An isolated
// outlier is re-stamped right after the last emitted frame; a jump that persists
// for a few frames is taken as a new timeline and joined to the previous one.
type ptsSanitizer struct {
	maxStep int64

	filled  bool
	lastRaw int64
	lastOut int64
	maxOut  int64
	step    int64

	outlierRaw   int64
	outlierOut   int64
	outlierCount int
}

func newPTSSanitizer(clockRate int) *ptsSanitizer {
	return &ptsSanitizer{maxStep: 5 * int64(clockRate)}
}

// sanitize returns the PTS to use for a packet. newFrame tells whether the
// previous packet closed a frame (RTP marker), so that two frames sharing a
// timestamp do not share a PTS.
func (s *ptsSanitizer) sanitize(raw int64, newFrame bool) int64 {
	if !s.filled {
		s.filled = true
		s.lastRaw = raw
		s.lastOut = raw
		s.maxOut = raw
		return raw
	}

	if raw == s.lastRaw {
		if newFrame {
			s.lastOut = max(s.lastOut, s.maxOut) + 1
		}
		return s.emit(s.lastOut)
	}

	if s.outlierCount > 0 && raw == s.outlierRaw {
		if newFrame {
			s.outlierOut = s.maxOut + 1
		}
		return s.emit(s.outlierOut)
	}

	diff := raw - s.lastRaw
	if diff >= -s.maxStep && diff <= s.maxStep {
		if diff > 0 {
			s.step = diff
		}
		s.lastRaw = raw
		s.lastOut += diff
		s.outlierCount = 0
		return s.emit(s.lastOut)
	}

	if s.outlierCount > 0 && abs64(raw-s.outlierRaw) <= s.maxStep {
		s.outlierCount++
	} else {
		s.outlierCount = 1
	}
	s.outlierRaw = raw

	if s.outlierCount >= ptsSanitizerJoinAfter {
		s.lastRaw = raw
		s.lastOut = max(s.lastOut+max(s.step, 1), s.maxOut+1)
		s.outlierCount = 0
		return s.emit(s.lastOut)
	}

	s.outlierOut = s.maxOut + 1
	return s.emit(s.outlierOut)
}

func (s *ptsSanitizer) emit(out int64) int64 {
	if out > s.maxOut {
		s.maxOut = out
	}
	return out
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
