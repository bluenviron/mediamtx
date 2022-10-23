package rpicamera

// Params is a set of camera parameters.
type Params struct {
	CameraID  int
	Width     int
	Height    int
	HFlip     bool
	VFlip     bool
	FPS       int
	IDRPeriod int
	Bitrate   int
	Profile   string
	Level     string
}
