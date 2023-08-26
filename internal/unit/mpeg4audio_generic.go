package unit

// MPEG4AudioGeneric is a MPEG-4 Audio data unit.
type MPEG4AudioGeneric struct {
	Base
	AUs [][]byte
}
