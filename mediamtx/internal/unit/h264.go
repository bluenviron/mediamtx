package unit

// H264 is a H264 data unit.
type H264 struct {
	Base
	AU [][]byte
}
