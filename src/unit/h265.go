package unit

// H265 is a H265 data unit.
type H265 struct {
	Base
	AU [][]byte
}
