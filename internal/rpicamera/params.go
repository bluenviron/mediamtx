package rpicamera

// Params is a set of camera parameters.
type Params struct {
	CameraID  int
	Width     int
	Height    int
	FPS       int
	IDRPeriod int
	Bitrate   int
	Profile   string
	Level     string
}
