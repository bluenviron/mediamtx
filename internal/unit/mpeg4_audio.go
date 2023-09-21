package unit

// MPEG4Audio is a MPEG-4 Audio data unit.
type MPEG4Audio struct {
	Base
	AUs [][]byte
}
