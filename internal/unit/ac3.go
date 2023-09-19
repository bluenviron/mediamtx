package unit

// AC3 is a AC-3 data unit.
type AC3 struct {
	Base
	Frames [][]byte
}
