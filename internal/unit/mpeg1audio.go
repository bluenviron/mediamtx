package unit

// MPEG1Audio is a MPEG-1/2 Audio data unit.
type MPEG1Audio struct {
	Base
	Frames [][]byte
}
