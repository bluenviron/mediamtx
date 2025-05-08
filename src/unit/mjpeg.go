package unit

// MJPEG is a M-JPEG data unit.
type MJPEG struct {
	Base
	Frame []byte
}
