package unit

// MPEG4Video is a MPEG-4 Video data unit.
type MPEG4Video struct {
	Base
	Frame []byte
}
