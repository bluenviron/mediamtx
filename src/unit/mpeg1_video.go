package unit

// MPEG1Video is a MPEG-1/2 Video data unit.
type MPEG1Video struct {
	Base
	Frame []byte
}
